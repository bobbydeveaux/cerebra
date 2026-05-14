package web

import (
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"path/filepath"
)

type orbData struct {
	BrainID      string
	ProjectName  string
	Summary      string
	Model        string
	GitBranch    string
	AgentType    string
	Status       string
	MessageCount int
	TokenUsage   int
	FirstMsg     string
	LastMsg      string
	OrbSize      int
	AnimDelay    float64
	Hue          int
}

func (s *Server) handleBrains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := s.store.GetBrainStats(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	brains, err := s.store.ListBrains(ctx, "", "", 0)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Convert to orb display data
	orbs := make([]orbData, 0, len(brains))
	for i, b := range brains {
		project := filepath.Base(b.ProjectPath)
		if project == "" || project == "." {
			project = b.ProjectKey
		}

		// Size: 60-180px based on message count (log scale for better distribution)
		size := 60
		if b.MessageCount > 0 {
			size = 60 + int(math.Min(120, math.Log2(float64(b.MessageCount+1))*12))
		}

		// Hue: hash project key for consistent colour per project
		h := fnv.New32a()
		h.Write([]byte(b.ProjectPath))
		hue := int(h.Sum32() % 360)

		// Animation delay: spread across cycle
		delay := float64(i%12) * 0.5

		summary := b.Summary
		if len(summary) > 80 {
			summary = summary[:77] + "..."
		}

		orbs = append(orbs, orbData{
			BrainID:      b.BrainID,
			ProjectName:  project,
			Summary:      summary,
			Model:        b.Model,
			GitBranch:    b.GitBranch,
			AgentType:    b.AgentType,
			Status:       b.Status,
			MessageCount: b.MessageCount,
			TokenUsage:   b.TokenUsage,
			FirstMsg:     b.FirstMessageAt,
			LastMsg:      b.LastMessageAt,
			OrbSize:      size,
			AnimDelay:    delay,
			Hue:          hue,
		})
	}

	data := map[string]interface{}{
		"Title":  "Agent Brains",
		"Stats":  stats,
		"Brains": orbs,
	}

	s.tmpls["brains.html"].ExecuteTemplate(w, "brains.html", data)
}

func (s *Server) handleBrainDetail(w http.ResponseWriter, r *http.Request) {
	brainID := r.PathValue("id")
	ctx := r.Context()

	b, err := s.store.GetBrain(ctx, brainID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if b == nil {
		http.Error(w, "Brain not found", 404)
		return
	}

	project := filepath.Base(b.ProjectPath)
	if project == "" || project == "." {
		project = b.ProjectKey
	}

	status := "Completed"
	statusClass := "status-completed"
	if b.Status == "active" {
		status = "Active"
		statusClass = "status-active"
	}

	tokens := formatTokenCount(b.TokenUsage)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<div class="detail-card">
    <div class="detail-header">
        <h2>%s</h2>
        <span class="detail-status %s">%s</span>
    </div>
    <div class="detail-grid">
        <div class="detail-item">
            <span class="detail-label">Brain ID</span>
            <span class="detail-value mono">%s</span>
        </div>
        <div class="detail-item">
            <span class="detail-label">Project Path</span>
            <span class="detail-value mono">%s</span>
        </div>
        <div class="detail-item">
            <span class="detail-label">Model</span>
            <span class="detail-value">%s</span>
        </div>
        <div class="detail-item">
            <span class="detail-label">Messages</span>
            <span class="detail-value">%d</span>
        </div>
        <div class="detail-item">
            <span class="detail-label">Tokens</span>
            <span class="detail-value">%s</span>
        </div>
        <div class="detail-item">
            <span class="detail-label">Branch</span>
            <span class="detail-value mono">%s</span>
        </div>
        <div class="detail-item">
            <span class="detail-label">Agent Type</span>
            <span class="detail-value">%s</span>
        </div>
        <div class="detail-item">
            <span class="detail-label">Version</span>
            <span class="detail-value">%s</span>
        </div>
        <div class="detail-item">
            <span class="detail-label">First Message</span>
            <span class="detail-value">%s</span>
        </div>
        <div class="detail-item">
            <span class="detail-label">Last Message</span>
            <span class="detail-value">%s</span>
        </div>
    </div>
    <div class="detail-summary">
        <span class="detail-label">Summary</span>
        <p>%s</p>
    </div>
</div>`,
		project, statusClass, status,
		b.BrainID, b.ProjectPath, b.Model,
		b.MessageCount, tokens, b.GitBranch,
		b.AgentType, b.Version,
		b.FirstMessageAt, b.LastMessageAt,
		b.Summary,
	)
}

func formatTokenCount(tokens int) string {
	if tokens >= 1_000_000_000 {
		return fmt.Sprintf("%.1fB", float64(tokens)/1_000_000_000)
	}
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	}
	if tokens >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1_000)
	}
	return fmt.Sprintf("%d", tokens)
}
