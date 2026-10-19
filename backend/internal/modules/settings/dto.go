package settings

type groupResponse struct {
	Group  string         `json:"group"`
	Values map[string]any `json:"values"`
}
