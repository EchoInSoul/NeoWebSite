package health

type Status struct {
	Status      string `json:"status"`
	Service     string `json:"service"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
	Database    string `json:"database"`
	Uptime      string `json:"uptime"`
	Time        string `json:"time"`
}
