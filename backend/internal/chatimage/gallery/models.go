package gallery

import "time"

const (
	defaultPageSize  = 24
	maxPageSize      = 100
	maxSelectedItems = 100
	maxImageBytes    = 64 * 1024 * 1024
	maxArchiveBytes  = 512 * 1024 * 1024
)

type Image struct {
	ID               int64     `json:"id"`
	URL              string    `json:"url"`
	DisplayName      string    `json:"display_name,omitempty"`
	MIMEType         string    `json:"mime_type,omitempty"`
	SizeBytes        *int64    `json:"size_bytes,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	MessageID        int64     `json:"message_id"`
	Model            string    `json:"model,omitempty"`
	ConversationID   int64     `json:"conversation_id"`
	ConversationName string    `json:"conversation_name"`
}

type ListResult struct {
	Items    []Image `json:"items"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
	Pages    int     `json:"pages"`
}

type DeleteResult struct {
	DeletedIDs  []int64  `json:"deleted_ids"`
	DeletedURLs []string `json:"deleted_urls"`
	Deleted     int      `json:"deleted"`
}

type SelectedImagesRequest struct {
	IDs []int64 `json:"ids" binding:"required"`
}
