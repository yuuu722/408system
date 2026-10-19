package domain

import (
	"context"
	"time"
)

type Conversation struct {
	ID, KnowledgeBaseID, Title string
	UpdatedAt                  time.Time
}
type Message struct {
	ID, Role, Content string
	Citations         []Citation
	CreatedAt         time.Time
}
type ConversationStore interface {
	ListConversations(context.Context, string) ([]Conversation, error)
	ConversationMessages(context.Context, string, string, string) ([]Message, error)
	RenameConversation(context.Context, string, string, string) error
	DeleteConversation(context.Context, string, string) error
	LockConversation(context.Context, string, string) (func(), error)
	SaveTurn(context.Context, string, string, string, string, []Citation) error
}

type StreamingLLM interface {
	AnswerStream(context.Context, string, string, []Message, func(string) error) (string, error)
}
