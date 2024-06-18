package entity

type Command struct {
	Table    string `json:"table"`
	Id       string `json:"id"`
	SerialNo string `json:"serialNo"`
	Command  []byte `json:"command"`
}

type BatchCommand struct {
	Table    string   `json:"table"`
	Ids      []string `json:"ids"`
	SerialNo string   `json:"serialNo"`
	Command  []byte   `json:"command"`
}

type CommandStatus string

const (
	COMMAND_STATUS_TIMEOUT CommandStatus = "timeout" // 超时
	COMMAND_STATUS_READY   CommandStatus = "ready"   // 待处理
	COMMAND_STATUS_REVOKE  CommandStatus = "revoke"  // 撤回
	COMMAND_STATUS_SUCCESS CommandStatus = "success" // 成功
	COMMAND_STATUS_FAIL    CommandStatus = "fail"    // 失败
)

type DriverInstruct struct {
	ID        string        `json:"id,omitempty"`
	Status    CommandStatus `json:"status,omitempty"`    // 状态
	RunResult interface{}   `json:"runResult,omitempty"` // 执行结果
}
