// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatwire

import "time"

// Bot is the bot the token belongs to.
type Bot struct {
	Capabilities []string `json:"capabilities,omitempty"`
	ID           string   `json:"bot_id"`
	Name         string   `json:"team"`
	TeamID       string   `json:"team_id"`
}

// Channel is one channel as the bot sees it.
type Channel struct {
	NativeID       string `json:"id,omitempty"`
	PostingAllowed *bool  `json:"posting_allowed,omitempty"`
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	IsPrivate      bool   `json:"is_private"`
	IsArchived     bool   `json:"is_archived"`
	BotIsMember    bool   `json:"bot_is_member"`
}

// Message carries the provider message and root identifiers. Seq preserves
// the ordered cursor used by existing Dispact links.
type Message struct {
	WriteRef   string
	NativeID   string
	RootID     string
	ChannelID  string
	Deleted    bool
	TS         string
	Seq        int64
	ThreadTS   string
	ReplyCount int
	UserName   string
	UserEmail  string
	Text       string
	PostedAt   time.Time
	EditedAt   time.Time
	Subtype    string
	IsBot      bool
	Files      []File
}

// File is a file attached to a message, as the history read lists it. Its
// content is never read.
type File struct {
	Name     string
	MimeType string
}

// HistoryPage is one page of a channel's history, oldest first.
type HistoryPage struct {
	HasMore    *bool
	Messages   []Message
	NextCursor *int64
}

// Posted is the chat's answer to a post. ID is the message's TS, the handle
// an edit names it by.
type Posted struct {
	ID  string
	Seq int64
}

// MemberResult says what happened to each email a channel was opened to.
type MemberResult struct {
	Added, AlreadyMember []string
	// NotInChat have no active chat account: unknown, a guest, or deactivated.
	NotInChat []string
	// NotAdded have an account the chat would not add: outside the app's
	// audience, a scope the bot lacks, or a failed add.
	NotAdded []string
}
