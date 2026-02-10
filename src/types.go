package app

const (
	defaultPollIntervalSeconds = 30
	defaultMaxPolls            = 120
	defaultOutputStaleSeconds  = 240
	defaultTmuxWidth           = 220
	defaultTmuxHeight          = 60
	maxInlineSendLength        = 500
	execDonePrefix             = "__LISA_EXEC_DONE__:"
)

type sessionMeta struct {
	Session     string `json:"session"`
	Agent       string `json:"agent"`
	Mode        string `json:"mode"`
	ProjectRoot string `json:"projectRoot"`
	StartCmd    string `json:"startCommand"`
	Prompt      string `json:"prompt,omitempty"`
	CreatedAt   string `json:"createdAt"`
}

type sessionState struct {
	PollCount         int    `json:"pollCount"`
	HasEverBeenActive bool   `json:"hasEverBeenActive"`
	LastOutputHash    string `json:"lastOutputHash"`
	LastOutputAt      int64  `json:"lastOutputAt"`
}

type sessionStatus struct {
	Session            string  `json:"session"`
	Agent              string  `json:"agent"`
	Mode               string  `json:"mode"`
	Status             string  `json:"status"`
	TodosDone          int     `json:"todosDone"`
	TodosTotal         int     `json:"todosTotal"`
	ActiveTask         string  `json:"activeTask"`
	WaitEstimate       int     `json:"waitEstimate"`
	SessionState       string  `json:"sessionState"`
	PaneStatus         string  `json:"paneStatus"`
	PaneCommand        string  `json:"paneCommand"`
	AgentPID           int     `json:"agentPid"`
	AgentCPU           float64 `json:"agentCpu"`
	OutputAgeSeconds   int     `json:"outputAgeSeconds"`
	OutputFreshSeconds int     `json:"outputFreshSeconds"`
	OutputFile         string  `json:"outputFile,omitempty"`
}

type monitorResult struct {
	FinalState  string `json:"finalState"`
	Session     string `json:"session"`
	TodosDone   int    `json:"todosDone"`
	TodosTotal  int    `json:"todosTotal"`
	OutputFile  string `json:"outputFile,omitempty"`
	ExitReason  string `json:"exitReason"`
	Polls       int    `json:"polls"`
	FinalStatus string `json:"finalStatus"`
}

type processInfo struct {
	PID     int
	PPID    int
	CPU     float64
	Command string
}
