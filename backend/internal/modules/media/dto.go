package media

type mediaResponse struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	SizeBytes    int64  `json:"size_bytes"`
	OriginalName string `json:"original_name"`
}

type deleteResponse struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}
