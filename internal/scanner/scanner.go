package scanner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/config"
)

type Category string

const (
	CategoryAPI        Category = "api"
	CategoryInfra      Category = "infrastructure"
	CategoryFrontend   Category = "frontend"
	CategoryTesting    Category = "testing"
	CategoryDocs       Category = "documentation"
	CategoryConfig     Category = "configuration"
	CategoryMigrations Category = "migrations"
	CategoryUnknown    Category = "unknown"
)

type FileType string

const (
	FileTypeCode         FileType = "code"
	FileTypeMarkdown     FileType = "markdown"
	FileTypeConfig       FileType = "config"
	FileTypeGitHistory   FileType = "git_history"
	FileTypeData         FileType = "data"
	FileTypeWiki         FileType = "wiki"
	FileTypeConversation FileType = "conversation"
)

type SourceType string

const (
	SourceTypeFilesystem    SourceType = "filesystem"
	SourceTypeConfluence    SourceType = "confluence"
	SourceTypeConversation  SourceType = "conversation"
)

type Document struct {
	ID          string
	Path        string
	RelPath     string
	Repo        string
	RepoRoot    string
	Category    Category
	Language    string
	FileType    FileType
	SourceType  SourceType
	Content     string
	ContentHash string
	ModTime     time.Time
	Metadata    map[string]string
}

type Scanner struct {
	cfg     *config.Config
	root    string
	ignorer *Ignorer
}

func New(cfg *config.Config) *Scanner {
	return &Scanner{
		cfg:     cfg,
		ignorer: NewIgnorer(cfg.Ignore),
	}
}

func (s *Scanner) Scan(ctx context.Context, root string) (<-chan Document, <-chan error) {
	s.root = root
	docs := make(chan Document, 100)
	errs := make(chan error, 10)

	go func() {
		defer close(docs)
		defer close(errs)

		absRoot, err := filepath.Abs(root)
		if err != nil {
			errs <- fmt.Errorf("resolving root path: %w", err)
			return
		}
		s.root = absRoot

		err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				select {
				case errs <- fmt.Errorf("walking %s: %w", path, err):
				default:
				}
				return nil
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			relPath, _ := filepath.Rel(absRoot, path)

			if d.IsDir() {
				if s.ignorer.ShouldIgnore(relPath, true) {
					return filepath.SkipDir
				}
				return nil
			}

			if s.ignorer.ShouldIgnore(relPath, false) {
				return nil
			}

			if isBinaryFile(path) {
				return nil
			}

			doc, err := s.scanFile(path, relPath)
			if err != nil {
				select {
				case errs <- fmt.Errorf("scanning %s: %w", relPath, err):
				default:
				}
				return nil
			}

			select {
			case docs <- *doc:
			case <-ctx.Done():
				return ctx.Err()
			}

			return nil
		})

		if err != nil {
			select {
			case errs <- err:
			default:
			}
		}
	}()

	return docs, errs
}

func (s *Scanner) ScanFile(path string) (*Document, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	relPath, _ := filepath.Rel(s.root, absPath)
	return s.scanFile(absPath, relPath)
}

func (s *Scanner) scanFile(absPath, relPath string) (*Document, error) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	contentStr := string(content)
	hash := fmt.Sprintf("%x", sha256.Sum256(content))
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(relPath)))

	lang := DetectLanguage(absPath)
	ft := DetectFileType(absPath, lang)
	cat := DetectCategory(relPath, lang, ft)

	repoName, repoRoot := findGitRepo(absPath)

	return &Document{
		ID:          id,
		Path:        absPath,
		RelPath:     relPath,
		Repo:        repoName,
		RepoRoot:    repoRoot,
		Category:    cat,
		Language:    lang,
		FileType:    ft,
		SourceType:  SourceTypeFilesystem,
		Content:     contentStr,
		ContentHash: hash,
		ModTime:     info.ModTime(),
		Metadata:    make(map[string]string),
	}, nil
}

func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return true
	}

	for _, b := range buf[:n] {
		if b == 0 {
			return true
		}
	}
	return false
}

func findGitRepo(path string) (string, string) {
	dir := filepath.Dir(path)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return filepath.Base(dir), dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", ""
}

type Ignorer struct {
	patterns []string
}

func NewIgnorer(patterns []string) *Ignorer {
	return &Ignorer{patterns: patterns}
}

func (ig *Ignorer) ShouldIgnore(relPath string, isDir bool) bool {
	base := filepath.Base(relPath)
	for _, pattern := range ig.patterns {
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
		if isDir && strings.HasPrefix(pattern, base) {
			if matched, _ := filepath.Match(pattern, base); matched {
				return true
			}
		}
	}
	return false
}
