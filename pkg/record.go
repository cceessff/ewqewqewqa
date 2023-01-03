package pkg

type Record struct {
	Id          int    `json:"id"`
	Domain      string `json:"domain"`
	Path        string `json:"path"`
	UserAgent   string `json:"user_agent"`
	Spider      string `json:"spider"`
	CreatedTime int64  `json:"created_time"`
}
type RecordParams struct {
	Page      int    `json:"page"`
	Limit     int    `json:"limit"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
	Domain    string `json:"domain"`
}
