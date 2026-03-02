package meetings

import (
	"time"
)

// Meeting represents a meeting record
type Meeting struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Status       MeetingStatus     `json:"status"`
	Participants []Participant     `json:"participants"`
	AgendaItems  []AgendaItem      `json:"agenda"`
	Transcript   []TranscriptEntry `json:"transcript"`
	ActionItems  []ActionItem      `json:"action_items"`
	Summary      string            `json:"summary"`
	CreatedAt    time.Time         `json:"created_at"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
}

// MeetingStatus represents the status of a meeting
type MeetingStatus string

const (
	MeetingStatusScheduled  MeetingStatus = "scheduled"
	MeetingStatusInProgress MeetingStatus = "in_progress"
	MeetingStatusCompleted  MeetingStatus = "completed"
	MeetingStatusCancelled  MeetingStatus = "cancelled"
)

// Participant represents a meeting participant
type Participant struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email,omitempty"`
	Role     string    `json:"role,omitempty"`
	JoinedAt time.Time `json:"joined_at"`
}

// AgendaItem represents an agenda item for a meeting
type AgendaItem struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Duration    int       `json:"duration_minutes,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	Order       int       `json:"order"`
	CreatedAt   time.Time `json:"created_at"`
}

// TranscriptEntry represents a single entry in the meeting transcript
type TranscriptEntry struct {
	ID        string    `json:"id"`
	Speaker   string    `json:"speaker"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ActionItem represents an action item created during a meeting
type ActionItem struct {
	ID        string     `json:"id"`
	BeadID    string     `json:"bead_id,omitempty"`
	Title     string     `json:"title"`
	Owner     string     `json:"owner,omitempty"`
	DueDate   *time.Time `json:"due_date,omitempty"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateMeetingRequest is the request to create a new meeting
type CreateMeetingRequest struct {
	Title        string        `json:"title"`
	Participants []Participant `json:"participants,omitempty"`
	AgendaItems  []AgendaItem  `json:"agenda,omitempty"`
}

// UpdateMeetingRequest is the request to update a meeting
type UpdateMeetingRequest struct {
	Title       *string        `json:"title,omitempty"`
	Status      *MeetingStatus `json:"status,omitempty"`
	Summary     *string        `json:"summary,omitempty"`
	AgendaItems []AgendaItem   `json:"agenda,omitempty"`
}

// AddTranscriptEntryRequest is the request to add a transcript entry
type AddTranscriptEntryRequest struct {
	Speaker string `json:"speaker"`
	Content string `json:"content"`
}

// AddActionItemRequest is the request to add an action item
type AddActionItemRequest struct {
	Title   string     `json:"title"`
	Owner   string     `json:"owner,omitempty"`
	DueDate *time.Time `json:"due_date,omitempty"`
}
