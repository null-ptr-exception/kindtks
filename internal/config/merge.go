package config

// MergeValues deep-merges override onto base: objects merge key by key,
// anything else in override replaces base. Inputs are not modified.
func MergeValues(base, override any) any {
	b, bok := base.(map[string]any)
	o, ook := override.(map[string]any)
	if !bok || !ook {
		return override
	}
	out := make(map[string]any, len(b)+len(o))
	for k, v := range b {
		out[k] = v
	}
	for k, v := range o {
		if bv, exists := out[k]; exists {
			out[k] = MergeValues(bv, v)
		} else {
			out[k] = v
		}
	}
	return out
}
