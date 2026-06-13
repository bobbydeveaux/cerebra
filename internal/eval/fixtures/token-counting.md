# Token Counting per Brain

Cerebra counts tokens per agent brain to estimate cost. An early bug
double-counted tokens: it summed cache_creation_input_tokens and
cache_read_input_tokens on top of input_tokens, which inflated the per-brain
total by between thirty and four hundred times.

The fix was to count line tokens as input_tokens plus output_tokens only,
excluding the cache fields from the per-brain total.
