package chatimage

import "time"

const (
	defaultConversationTitle = "新对话"
	maxTitleRunes            = 200
	maxContentBytes          = 200_000
	maxModelRunes            = 128
	maxResourcesPerMessage   = 16
	maxResourcePathBytes     = 8192
	maxDisplayNameRunes      = 255
	maxMIMETypeRunes         = 128
)

type Resource struct {
	ID          int64     `json:"id"`
	Type        string    `json:"type"`
	URL         string    `json:"url"`
	DisplayName string    `json:"display_name,omitempty"`
	MIMEType    string    `json:"mime_type,omitempty"`
	SizeBytes   *int64    `json:"size_bytes,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Message struct {
	ID             int64      `json:"id"`
	ClientID       string     `json:"client_id"`
	ConversationID int64      `json:"conversation_id"`
	Role           string     `json:"role"`
	Kind           string     `json:"kind"`
	Content        string     `json:"content"`
	Model          string     `json:"model,omitempty"`
	Resources      []Resource `json:"resources"`
	CreatedAt      time.Time  `json:"created_at"`
}

type ConversationSummary struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	MessageCount int64     `json:"message_count"`
	Preview      string    `json:"preview,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Conversation struct {
	ConversationSummary
	Messages []Message `json:"messages"`
}

type ResourceInput struct {
	Type        string `json:"type"`
	URL         string `json:"url"`
	DisplayName string `json:"display_name,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
	SizeBytes   *int64 `json:"size_bytes,omitempty"`
}

type MessageInput struct {
	ClientID  string          `json:"client_id"`
	Role      string          `json:"role"`
	Kind      string          `json:"kind"`
	Content   string          `json:"content"`
	Model     string          `json:"model,omitempty"`
	Resources []ResourceInput `json:"resources,omitempty"`
}

type AppendMessageResult struct {
	Message      Message   `json:"message"`
	Title        string    `json:"title"`
	MessageCount int64     `json:"message_count"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ListResult struct {
	Items    []ConversationSummary `json:"items"`
	Total    int64                 `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	Pages    int                   `json:"pages"`
}
