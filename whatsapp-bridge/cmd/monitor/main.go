// Command monitor collects plain live messages for explicitly allowed chats.
// It intentionally imports none of the stock collector's routing code.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type chats map[string]bool

func (c chats) String() string { return "[redacted]" }
func (c chats) Set(value string) error {
	jid, err := types.ParseJID(value)
	if err != nil || jid.User == "" || jid.String() != value || jid.Device != 0 ||
		(jid.Server != types.DefaultUserServer && jid.Server != types.HiddenUserServer && jid.Server != types.GroupServer) {
		return errors.New("expected an exact private or group chat JID")
	}
	c[value] = true
	return nil
}

func record(db *sql.DB, allowed chats, e *events.Message) error {
	chat := e.Info.Chat.String()
	if !allowed[chat] || e.Message == nil || e.Info.ID == "" {
		return nil
	}
	if p := e.Message.GetProtocolMessage(); p != nil {
		if p.GetKey() == nil || p.GetType().String() != "REVOKE" {
			return nil
		}
		_, err := db.Exec("DELETE FROM messages WHERE chat_jid=? AND id=?", chat, p.GetKey().GetID())
		return err
	}
	// Disappearing, view-once, and media messages are not collected.
	if e.IsEphemeral || e.IsViewOnce || e.IsViewOnceV2 || e.IsViewOnceV2Extension {
		return nil
	}
	if x := e.Message.GetExtendedTextMessage(); x != nil &&
		(x.GetContextInfo().GetExpiration() != 0 || x.GetContextInfo().GetEphemeralSettingTimestamp() != 0) {
		return nil
	}
	body := e.Message.GetConversation()
	if body == "" {
		body = e.Message.GetExtendedTextMessage().GetText()
	}
	body = strings.ToValidUTF8(body, "")
	if body == "" {
		return nil
	}
	_, err := db.Exec(
		"INSERT OR IGNORE INTO messages(id, chat_jid, sender, content, timestamp, is_from_me) VALUES(?,?,?,?,?,?)",
		string(e.Info.ID), chat, e.Info.Sender.String(), body, e.Info.Timestamp.UTC().Format("2006-01-02T15:04:05.000000000Z"), e.Info.IsFromMe)
	return err
}

func collect(ctx context.Context, dir string, allowed chats, pair bool) error {
	if len(allowed) == 0 {
		return errors.New("at least one --allow-chat is required")
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return errors.New("cannot create private monitor directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return errors.New("monitor directory must be a real owner-only directory")
	}
	for _, name := range []string{"session.db", "messages.db"} {
		path := filepath.Join(dir, name)
		if st, err := os.Lstat(path); err == nil {
			if !st.Mode().IsRegular() || st.Mode().Perm() != 0o600 || st.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
				return errors.New("monitor databases must be real owner-only files")
			}
		} else if !os.IsNotExist(err) {
			return errors.New("cannot inspect monitor database")
		}
	}
	session, err := sqlstore.New(ctx, "sqlite3", filepath.Join(dir, "session.db")+"?_foreign_keys=on", nil)
	if err != nil {
		return errors.New("cannot open protocol session store")
	}
	defer session.Close()
	device, err := session.GetFirstDevice(ctx)
	if err != nil {
		return errors.New("cannot load protocol session")
	}
	fresh := device.ID == nil
	var terminal *os.File
	if fresh {
		if !pair {
			return errors.New("not paired; explicit --pair from a private terminal is required")
		}
		terminal, err = os.OpenFile("/dev/tty", os.O_WRONLY, 0)
		if err != nil {
			return errors.New("pairing requires a private local terminal")
		}
		defer terminal.Close()
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "messages.db")+"?_busy_timeout=2000")
	if err != nil {
		return errors.New("cannot open message store")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS messages (
		id TEXT NOT NULL, chat_jid TEXT NOT NULL, sender TEXT NOT NULL, content TEXT NOT NULL,
		timestamp TEXT NOT NULL, is_from_me INTEGER NOT NULL, deleted_at TEXT,
		PRIMARY KEY (chat_jid,id))`); err != nil {
		return errors.New("cannot initialize message store")
	}
	client := whatsmeow.NewClient(device, nil)
	client.ManualHistorySyncDownload = true
	client.DisableManualHistorySyncReceipt = true
	client.AutomaticMessageRerequestFromPhone = false
	client.EnableAutoReconnect = false
	client.DisableLoginAutoReconnect = true
	client.SetForceActiveDeliveryReceipts(false)
	run, stop := context.WithCancel(ctx)
	defer stop()
	client.AddEventHandler(func(evt any) {
		switch e := evt.(type) {
		case *events.Message:
			if err := record(db, allowed, e); err != nil {
				fmt.Fprintln(os.Stderr, "Message store write failed; stopping.")
				stop()
			}
		case *events.LoggedOut, *events.Disconnected, *events.ClientOutdated:
			stop()
		}
	})
	if fresh {
		qr, err := client.GetQRChannel(run)
		if err != nil {
			return errors.New("cannot initiate private local pairing")
		}
		go func() {
			for item := range qr {
				switch item.Event {
				case "code":
					qrterminal.GenerateHalfBlock(item.Code, qrterminal.L, terminal)
				case "success":
					return
				default:
					stop()
					return
				}
			}
		}()
	}
	if err := client.ConnectContext(run); err != nil {
		return errors.New("WhatsApp protocol connection failed")
	}
	defer client.Disconnect()
	<-run.Done()
	return nil
}

func main() {
	syscall.Umask(0o077)
	allowed := chats{}
	dir := flag.String("store", "", "new dedicated owner-only monitor directory (absolute path)")
	pair := flag.Bool("pair", false, "allow initial QR pairing on the local controlling terminal")
	flag.Var(allowed, "allow-chat", "exact chat JID; repeat for additional chats")
	flag.Parse()
	if !filepath.IsAbs(*dir) || strings.ContainsAny(*dir, "?&#") {
		fmt.Fprintln(os.Stderr, "Specify an absolute --store path without SQLite URI characters.")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := collect(ctx, *dir, allowed, *pair); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
