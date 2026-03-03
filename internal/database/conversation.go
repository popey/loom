package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jordanhubbard/loom/pkg/models"
)

func (d *Database) CreateConversationContext(ctx *models.ConversationContext) error {
	messagesJSON, err := ctx.MessagesJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal messages: %w", err)
	}

	metadataJSON, err := ctx.MetadataJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := "INSERT INTO conversation_contexts (session_id, bead_id, project_id, messages, created_at, updated_at, expires_at, token_count, metadata) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"

	_, err = d.db.Exec(rebind(query),
		ctx.SessionID,
		ctx.BeadID,
		ctx.ProjectID,
		messagesJSON,
		ctx.CreatedAt,
		ctx.UpdatedAt,
		ctx.ExpiresAt,
		ctx.TokenCount,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to create conversation context: %w", err)
	}
	return nil
}

func initializeEntityMetadata(ctx *models.ConversationContext) {
	if ctx.EntityMetadata.Attributes == nil {
		ctx.EntityMetadata = models.NewEntityMetadata(models.ConversationSchemaVersion)
	}
}

func (d *Database) GetConversationContext(sessionID string) (*models.ConversationContext, error) {
	query := "SELECT session_id, bead_id, project_id, messages, created_at, updated_at, expires_at, token_count, metadata FROM conversation_contexts WHERE session_id = ?"

	ctx := &models.ConversationContext{}
	var messagesJSON, metadataJSON []byte

	err := d.db.QueryRow(rebind(query), sessionID).Scan(
		&ctx.SessionID,
		&ctx.BeadID,
		&ctx.ProjectID,
		&messagesJSON,
		&ctx.CreatedAt,
		&ctx.UpdatedAt,
		&ctx.ExpiresAt,
		&ctx.TokenCount,
		&metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("conversation context not found: %s", sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation context: %w", err)
	}

	if err := ctx.SetMessagesFromJSON(messagesJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal messages: %w", err)
	}
	if err := ctx.SetMetadataFromJSON(metadataJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	initializeEntityMetadata(ctx)

	return ctx, nil
}

func (d *Database) GetConversationContextByBeadID(beadID string) (*models.ConversationContext, error) {
	query := "SELECT session_id, bead_id, project_id, messages, created_at, updated_at, expires_at, token_count, metadata FROM conversation_contexts WHERE bead_id = ? ORDER BY updated_at DESC LIMIT 1"

	ctx := &models.ConversationContext{}
	var messagesJSON, metadataJSON []byte

	err := d.db.QueryRow(rebind(query), beadID).Scan(
		&ctx.SessionID,
		&ctx.BeadID,
		&ctx.ProjectID,
		&messagesJSON,
		&ctx.CreatedAt,
		&ctx.UpdatedAt,
		&ctx.ExpiresAt,
		&ctx.TokenCount,
		&metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("conversation context not found for bead: %s", beadID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation context: %w", err)
	}

	if err := ctx.SetMessagesFromJSON(messagesJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal messages: %w", err)
	}
	if err := ctx.SetMetadataFromJSON(metadataJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	initializeEntityMetadata(ctx)

	return ctx, nil
}

func (d *Database) InjectMessageIntoConversation(sessionID string, message string) error {
	ctx, err := d.GetConversationContext(sessionID)
	if err != nil {
		return err
	}

	newMessage := models.ChatMessage{
		Role:      "user",
		Content:   message,
		Timestamp: time.Now(),
	}

	ctx.Messages = append(ctx.Messages, newMessage)
	ctx.TokenCount += newMessage.TokenCount
	ctx.UpdatedAt = time.Now()

	return d.UpdateConversationContext(ctx)
}

func (d *Database) UpdateConversationContext(ctx *models.ConversationContext) error {
	messagesJSON, err := ctx.MessagesJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal messages: %w", err)
	}

	metadataJSON, err := ctx.MetadataJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := "UPDATE conversation_contexts SET messages = ?, updated_at = ?, token_count = ?, metadata = ? WHERE session_id = ?"

	result, err := d.db.Exec(rebind(query),
		messagesJSON,
		ctx.UpdatedAt,
		ctx.TokenCount,
		metadataJSON,
		ctx.SessionID,
	)

	if err != nil {
		return fmt.Errorf("failed to update conversation context: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("conversation context not found: %s", ctx.SessionID)
	}

	return nil
}

func (d *Database) DeleteConversationContext(sessionID string) error {
	query := "DELETE FROM conversation_contexts WHERE session_id = ?"

	result, err := d.db.Exec(rebind(query), sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete conversation context: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("conversation context not found: %s", sessionID)
	}

	return nil
}

func (d *Database) DeleteExpiredConversationContexts() (int64, error) {
	query := "DELETE FROM conversation_contexts WHERE expires_at < ?"

	result, err := d.db.Exec(rebind(query), time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired conversations: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rows, nil
}

func (d *Database) ListConversationContextsByProject(queryCtx context.Context, projectID string, limit int) ([]*models.ConversationContext, error) {
	query := "SELECT session_id, bead_id, project_id, messages, created_at, updated_at, expires_at, token_count, metadata FROM conversation_contexts WHERE project_id = ? ORDER BY updated_at DESC LIMIT ?"

	rows, err := d.db.QueryContext(queryCtx, rebind(query), projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list conversation contexts: %w", err)
	}
	defer rows.Close()

	var contexts []*models.ConversationContext
	for rows.Next() {
		convCtx := &models.ConversationContext{}
		var messagesJSON, metadataJSON []byte

		err := rows.Scan(
			&convCtx.SessionID,
			&convCtx.BeadID,
			&convCtx.ProjectID,
			&messagesJSON,
			&convCtx.CreatedAt,
			&convCtx.UpdatedAt,
			&convCtx.ExpiresAt,
			&convCtx.TokenCount,
			&metadataJSON,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan conversation context: %w", err)
		}

		if err := convCtx.SetMessagesFromJSON(messagesJSON); err != nil {
			return nil, fmt.Errorf("failed to unmarshal messages: %w", err)
		}
		if err := convCtx.SetMetadataFromJSON(metadataJSON); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		initializeEntityMetadata(convCtx)

		contexts = append(contexts, convCtx)
	}

	// Check for errors that occurred during iteration
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating conversation contexts: %w", err)
	}

	return contexts, nil
}

func (d *Database) ResetConversationMessages(sessionID string, keepSystemMessage bool) error {
	ctx, err := d.GetConversationContext(sessionID)
	if err != nil {
		return err
	}

	var newMessages []models.ChatMessage
	if keepSystemMessage && len(ctx.Messages) > 0 && ctx.Messages[0].Role == "system" {
		newMessages = []models.ChatMessage{ctx.Messages[0]}
	}

	ctx.Messages = newMessages
	ctx.TokenCount = 0
	for _, msg := range newMessages {
		ctx.TokenCount += msg.TokenCount
	}
	ctx.UpdatedAt = time.Now()

	return d.UpdateConversationContext(ctx)
}
