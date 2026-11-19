package main

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestRecordSynthetic(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE TABLE messages (id TEXT, chat_jid TEXT, sender TEXT, content TEXT, timestamp TEXT, is_from_me INTEGER, PRIMARY KEY(chat_jid,id))"); err != nil {
		t.Fatal(err)
	}
	allowed := chats{}
	if err := allowed.Set("123@s.whatsapp.net"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "status@broadcast", "123.1:2@s.whatsapp.net", "123@unknown", "s.whatsapp.net"} {
		if err := allowed.Set(s); err == nil {
			t.Fatal("invalid allowlist entry accepted")
		}
	}
	direct := types.NewJID("123", types.DefaultUserServer)
	other := types.NewJID("456", types.DefaultUserServer)
	message := func(chat types.JID, id string, body *waE2E.Message) *events.Message {
		return &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, Sender: chat}, ID: id, Timestamp: time.Now()}, Message: body}
	}
	ephemeral := message(direct, "e", &waE2E.Message{Conversation: proto.String("ignore")})
	ephemeral.IsEphemeral = true
	for _, e := range []*events.Message{
		message(direct, "a", &waE2E.Message{Conversation: proto.String("synthetic")}),
		message(direct, "a", &waE2E.Message{Conversation: proto.String("duplicate")}),
		message(other, "b", &waE2E.Message{Conversation: proto.String("ignore")}),
		message(direct, "c", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("ignore")}}),
		message(direct, "d", &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("ignore"), ContextInfo: &waE2E.ContextInfo{Expiration: proto.Uint32(30)}}}),
		ephemeral,
	} {
		if err := record(db, allowed, e); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	var body string
	if err := db.QueryRow("SELECT COUNT(*), MAX(content) FROM messages").Scan(&count, &body); err != nil || count != 1 || body != "synthetic" {
		t.Fatalf("unexpected records: %d %s %v", count, body, err)
	}
}

func TestEmptyScopeFailsBeforeStorage(t *testing.T) {
	err := collect(t.Context(), "/path-not-used", chats{}, false)
	if err == nil || !strings.Contains(err.Error(), "--allow-chat") {
		t.Fatal("expected scope error")
	}
}
