package models

type LoginRequestModel struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Version  string `json:"version,omitempty"`
}

type LoginResponseModel struct {
	Username string `json:"username,omitempty"`
	Success bool   `json:"success,omitempty"`
	Message string `json:"message,omitempty"`
	Blocks  int    `json:"blocks,omitempty"`
	MsgCost int    `json:"msgCost,omitempty"`
}
