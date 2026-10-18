package gallery

import (
	"context"
	"database/sql"
	"fmt"
)

type SQLRepository struct {
	db *sql.DB
}

func NewSQLRepository(db *sql.DB) *SQLRepository {
	return &SQLRepository{db: db}
}

func (r *SQLRepository) ListImages(ctx context.Context, userID int64, page, pageSize int) ([]Image, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM chat_image_resources r
		JOIN chat_image_messages m ON m.id = r.message_id
		JOIN chat_image_conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND m.role = 'assistant' AND r.resource_type = 'image'
	`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count chat image gallery resources: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.resource_path, r.display_name, r.mime_type, r.size_bytes, r.created_at,
		       m.id, m.model, c.id, c.title
		FROM chat_image_resources r
		JOIN chat_image_messages m ON m.id = r.message_id
		JOIN chat_image_conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND m.role = 'assistant' AND r.resource_type = 'image'
		ORDER BY r.created_at DESC, r.id DESC
		LIMIT $2 OFFSET $3
	`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list chat image gallery resources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Image, 0, pageSize)
	for rows.Next() {
		var item Image
		var sizeBytes sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.URL, &item.DisplayName, &item.MIMEType, &sizeBytes, &item.CreatedAt,
			&item.MessageID, &item.Model, &item.ConversationID, &item.ConversationName,
		); err != nil {
			return nil, 0, fmt.Errorf("scan chat image gallery resource: %w", err)
		}
		if sizeBytes.Valid {
			value := sizeBytes.Int64
			item.SizeBytes = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate chat image gallery resources: %w", err)
	}
	return items, total, nil
}

func (r *SQLRepository) GetImages(ctx context.Context, userID int64, ids []int64) ([]Image, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.resource_path, r.display_name, r.mime_type, r.size_bytes, r.created_at,
		       m.id, m.model, c.id, c.title
		FROM chat_image_resources r
		JOIN chat_image_messages m ON m.id = r.message_id
		JOIN chat_image_conversations c ON c.id = m.conversation_id
		WHERE c.user_id = $1 AND m.role = 'assistant' AND r.resource_type = 'image'
		  AND r.id = ANY($2)
		ORDER BY array_position($2::bigint[], r.id)
	`, userID, ids)
	if err != nil {
		return nil, fmt.Errorf("get chat image gallery resources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]Image, 0, len(ids))
	for rows.Next() {
		var item Image
		var sizeBytes sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.URL, &item.DisplayName, &item.MIMEType, &sizeBytes, &item.CreatedAt,
			&item.MessageID, &item.Model, &item.ConversationID, &item.ConversationName,
		); err != nil {
			return nil, fmt.Errorf("scan selected chat image gallery resource: %w", err)
		}
		if sizeBytes.Valid {
			value := sizeBytes.Int64
			item.SizeBytes = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate selected chat image gallery resources: %w", err)
	}
	return items, nil
}

func (r *SQLRepository) DeleteImages(ctx context.Context, userID int64, ids []int64) (*DeleteResult, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH selected_urls AS (
			SELECT DISTINCT r.resource_path
			FROM chat_image_resources r
			JOIN chat_image_messages m ON m.id = r.message_id
			JOIN chat_image_conversations c ON c.id = m.conversation_id
			WHERE c.user_id = $1 AND m.role = 'assistant' AND r.resource_type = 'image'
			  AND r.id = ANY($2)
		), deleted AS (
			DELETE FROM chat_image_resources r
			USING chat_image_messages m, chat_image_conversations c, selected_urls s
			WHERE r.message_id = m.id AND m.conversation_id = c.id AND c.user_id = $1
			  AND r.resource_path = s.resource_path
			RETURNING r.id, r.resource_path
		)
		SELECT id, resource_path FROM deleted ORDER BY id
	`, userID, ids)
	if err != nil {
		return nil, fmt.Errorf("delete chat image gallery resources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := &DeleteResult{DeletedIDs: []int64{}, DeletedURLs: []string{}}
	seenURLs := make(map[string]struct{})
	for rows.Next() {
		var id int64
		var resourceURL string
		if err := rows.Scan(&id, &resourceURL); err != nil {
			return nil, fmt.Errorf("scan deleted chat image gallery resource: %w", err)
		}
		result.DeletedIDs = append(result.DeletedIDs, id)
		if _, exists := seenURLs[resourceURL]; !exists {
			seenURLs[resourceURL] = struct{}{}
			result.DeletedURLs = append(result.DeletedURLs, resourceURL)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deleted chat image gallery resources: %w", err)
	}
	result.Deleted = len(result.DeletedIDs)
	return result, nil
}
