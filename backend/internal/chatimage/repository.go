package chatimage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type SQLRepository struct {
	db *sql.DB
}

var errConversationNotFound = errors.New("conversation not found")

func NewSQLRepository(db *sql.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) ListConversations(ctx context.Context, userID int64, page, pageSize int, search string) ([]ConversationSummary, int64, error) {
	pattern := "%" + search + "%"
	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_image_conversations
		WHERE user_id = $1 AND ($2 = '' OR title ILIKE $3)
	`, userID, search, pattern).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count chat image conversations: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.title,
		       (SELECT COUNT(*) FROM chat_image_messages m WHERE m.conversation_id = c.id) AS message_count,
		       COALESCE((
		           SELECT NULLIF(LEFT(m.content, 160), '')
		           FROM chat_image_messages m
		           WHERE m.conversation_id = c.id
		           ORDER BY m.created_at DESC, m.id DESC
		           LIMIT 1
		       ), '') AS preview,
		       c.created_at, c.updated_at
		FROM chat_image_conversations c
		WHERE c.user_id = $1 AND ($2 = '' OR c.title ILIKE $3)
		ORDER BY c.updated_at DESC, c.id DESC
		LIMIT $4 OFFSET $5
	`, userID, search, pattern, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list chat image conversations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]ConversationSummary, 0, pageSize)
	for rows.Next() {
		var item ConversationSummary
		if err := rows.Scan(&item.ID, &item.Title, &item.MessageCount, &item.Preview, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan chat image conversation: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate chat image conversations: %w", err)
	}
	return items, total, nil
}

func (r *SQLRepository) GetConversation(ctx context.Context, userID, conversationID int64) (*Conversation, error) {
	summary, err := r.getSummary(ctx, r.db, userID, conversationID)
	if err != nil {
		return nil, err
	}
	conversation := &Conversation{ConversationSummary: *summary, Messages: []Message{}}

	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.client_id, m.conversation_id, m.role, m.kind, m.content, m.model, m.created_at,
		       r.id, r.resource_type, r.resource_path, r.display_name, r.mime_type, r.size_bytes, r.created_at
		FROM chat_image_messages m
		LEFT JOIN chat_image_resources r ON r.message_id = m.id
		WHERE m.conversation_id = $1
		ORDER BY m.created_at ASC, m.id ASC, r.id ASC
	`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list chat image messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	messageIndex := make(map[int64]int)
	for rows.Next() {
		var message Message
		var resourceID sql.NullInt64
		var resourceType, resourcePath, displayName, mimeType sql.NullString
		var sizeBytes sql.NullInt64
		var resourceCreatedAt sql.NullTime
		if err := rows.Scan(
			&message.ID, &message.ClientID, &message.ConversationID, &message.Role, &message.Kind, &message.Content, &message.Model, &message.CreatedAt,
			&resourceID, &resourceType, &resourcePath, &displayName, &mimeType, &sizeBytes, &resourceCreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan chat image message: %w", err)
		}
		index, exists := messageIndex[message.ID]
		if !exists {
			message.Resources = []Resource{}
			conversation.Messages = append(conversation.Messages, message)
			index = len(conversation.Messages) - 1
			messageIndex[message.ID] = index
		}
		if resourceID.Valid {
			resource := Resource{
				ID:          resourceID.Int64,
				Type:        resourceType.String,
				URL:         resourcePath.String,
				DisplayName: displayName.String,
				MIMEType:    mimeType.String,
				CreatedAt:   resourceCreatedAt.Time,
			}
			if sizeBytes.Valid {
				value := sizeBytes.Int64
				resource.SizeBytes = &value
			}
			conversation.Messages[index].Resources = append(conversation.Messages[index].Resources, resource)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat image messages: %w", err)
	}
	return conversation, nil
}

func (r *SQLRepository) CreateConversation(ctx context.Context, userID int64, title string) (*Conversation, error) {
	conversation := &Conversation{Messages: []Message{}}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO chat_image_conversations (user_id, title)
		VALUES ($1, $2)
		RETURNING id, title, created_at, updated_at
	`, userID, title).Scan(
		&conversation.ID, &conversation.Title, &conversation.CreatedAt, &conversation.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create chat image conversation: %w", err)
	}
	return conversation, nil
}

func (r *SQLRepository) RenameConversation(ctx context.Context, userID, conversationID int64, title string) (*ConversationSummary, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE chat_image_conversations
		SET title = $3, updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`, conversationID, userID, title)
	if err != nil {
		return nil, fmt.Errorf("rename chat image conversation: %w", err)
	}
	if err := requireAffected(result); err != nil {
		return nil, err
	}
	return r.getSummary(ctx, r.db, userID, conversationID)
}

func (r *SQLRepository) DeleteConversation(ctx context.Context, userID, conversationID int64) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM chat_image_conversations WHERE id = $1 AND user_id = $2
	`, conversationID, userID)
	if err != nil {
		return fmt.Errorf("delete chat image conversation: %w", err)
	}
	return requireAffected(result)
}

func (r *SQLRepository) ClearConversation(ctx context.Context, userID, conversationID int64) (*ConversationSummary, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin clear chat image conversation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE chat_image_conversations
		SET title = $3, updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`, conversationID, userID, defaultConversationTitle)
	if err != nil {
		return nil, fmt.Errorf("reset chat image conversation: %w", err)
	}
	if err := requireAffected(result); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chat_image_messages WHERE conversation_id = $1`, conversationID); err != nil {
		return nil, fmt.Errorf("clear chat image messages: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit clear chat image conversation: %w", err)
	}
	return r.getSummary(ctx, r.db, userID, conversationID)
}

func (r *SQLRepository) AppendMessage(ctx context.Context, userID, conversationID int64, input MessageInput) (*AppendMessageResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin append chat image message: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentTitle string
	if err := tx.QueryRowContext(ctx, `
		SELECT title FROM chat_image_conversations
		WHERE id = $1 AND user_id = $2
		FOR UPDATE
	`, conversationID, userID).Scan(&currentTitle); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errConversationNotFound
		}
		return nil, fmt.Errorf("lock chat image conversation: %w", err)
	}

	result := &AppendMessageResult{Message: Message{ConversationID: conversationID, Resources: []Resource{}}}
	var inserted bool
	err = tx.QueryRowContext(ctx, `
		WITH inserted AS (
			INSERT INTO chat_image_messages (conversation_id, client_id, role, kind, content, model)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (conversation_id, client_id) DO NOTHING
			RETURNING id, client_id, role, kind, content, model, created_at
		)
		SELECT id, client_id, role, kind, content, model, created_at, TRUE
		FROM inserted
		UNION ALL
		SELECT id, client_id, role, kind, content, model, created_at, FALSE
		FROM chat_image_messages
		WHERE conversation_id = $1 AND client_id = $2
		  AND NOT EXISTS (SELECT 1 FROM inserted)
		LIMIT 1
	`, conversationID, input.ClientID, input.Role, input.Kind, input.Content, input.Model).Scan(
		&result.Message.ID, &result.Message.ClientID, &result.Message.Role, &result.Message.Kind, &result.Message.Content, &result.Message.Model, &result.Message.CreatedAt, &inserted,
	)
	if err != nil {
		return nil, fmt.Errorf("append chat image message: %w", err)
	}

	for _, inputResource := range input.Resources {
		if !inserted {
			break
		}
		resource := Resource{
			Type:        inputResource.Type,
			URL:         inputResource.URL,
			DisplayName: inputResource.DisplayName,
			MIMEType:    inputResource.MIMEType,
			SizeBytes:   inputResource.SizeBytes,
		}
		err = tx.QueryRowContext(ctx, `
			INSERT INTO chat_image_resources
			    (message_id, resource_type, resource_path, display_name, mime_type, size_bytes)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (message_id, md5(resource_path)) DO UPDATE SET
			    resource_type = EXCLUDED.resource_type,
			    display_name = EXCLUDED.display_name,
			    mime_type = EXCLUDED.mime_type,
			    size_bytes = EXCLUDED.size_bytes
			RETURNING id, created_at
		`, result.Message.ID, resource.Type, resource.URL, resource.DisplayName, resource.MIMEType, resource.SizeBytes).Scan(
			&resource.ID, &resource.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("append chat image resource: %w", err)
		}
		result.Message.Resources = append(result.Message.Resources, resource)
	}
	if !inserted {
		rows, queryErr := tx.QueryContext(ctx, `
			SELECT id, resource_type, resource_path, display_name, mime_type, size_bytes, created_at
			FROM chat_image_resources
			WHERE message_id = $1
			ORDER BY id ASC
		`, result.Message.ID)
		if queryErr != nil {
			return nil, fmt.Errorf("list idempotent chat image resources: %w", queryErr)
		}
		for rows.Next() {
			var resource Resource
			var sizeBytes sql.NullInt64
			if scanErr := rows.Scan(
				&resource.ID, &resource.Type, &resource.URL, &resource.DisplayName,
				&resource.MIMEType, &sizeBytes, &resource.CreatedAt,
			); scanErr != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan idempotent chat image resource: %w", scanErr)
			}
			if sizeBytes.Valid {
				value := sizeBytes.Int64
				resource.SizeBytes = &value
			}
			result.Message.Resources = append(result.Message.Resources, resource)
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterate idempotent chat image resources: %w", rowsErr)
		}
		if closeErr := rows.Close(); closeErr != nil {
			return nil, fmt.Errorf("close idempotent chat image resources: %w", closeErr)
		}
	}

	if inserted {
		nextTitle := currentTitle
		if currentTitle == defaultConversationTitle && result.Message.Role == "user" {
			nextTitle = titleFromMessage(result.Message.Content)
		}
		err = tx.QueryRowContext(ctx, `
			UPDATE chat_image_conversations
			SET title = $3, updated_at = NOW()
			WHERE id = $1 AND user_id = $2
			RETURNING title, updated_at
		`, conversationID, userID, nextTitle).Scan(&result.Title, &result.UpdatedAt)
	} else {
		err = tx.QueryRowContext(ctx, `
			SELECT title, updated_at
			FROM chat_image_conversations
			WHERE id = $1 AND user_id = $2
		`, conversationID, userID).Scan(&result.Title, &result.UpdatedAt)
	}
	if err != nil {
		return nil, fmt.Errorf("touch chat image conversation: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM chat_image_messages WHERE conversation_id = $1
	`, conversationID).Scan(&result.MessageCount); err != nil {
		return nil, fmt.Errorf("count chat image messages: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit append chat image message: %w", err)
	}
	return result, nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *SQLRepository) getSummary(ctx context.Context, db queryRower, userID, conversationID int64) (*ConversationSummary, error) {
	var item ConversationSummary
	err := db.QueryRowContext(ctx, `
		SELECT c.id, c.title,
		       (SELECT COUNT(*) FROM chat_image_messages m WHERE m.conversation_id = c.id),
		       COALESCE((
		           SELECT NULLIF(LEFT(m.content, 160), '') FROM chat_image_messages m
		           WHERE m.conversation_id = c.id
		           ORDER BY m.created_at DESC, m.id DESC LIMIT 1
		       ), ''),
		       c.created_at, c.updated_at
		FROM chat_image_conversations c
		WHERE c.id = $1 AND c.user_id = $2
	`, conversationID, userID).Scan(
		&item.ID, &item.Title, &item.MessageCount, &item.Preview, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errConversationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get chat image conversation: %w", err)
	}
	return &item, nil
}

func requireAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read chat image affected rows: %w", err)
	}
	if affected == 0 {
		return errConversationNotFound
	}
	return nil
}

func titleFromMessage(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "图片对话"
	}
	if utf8.RuneCountInString(content) <= 28 {
		return content
	}
	runes := []rune(content)
	return string(runes[:28])
}
