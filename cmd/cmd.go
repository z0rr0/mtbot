// Package cmd contains bot commands' handlers.
package cmd

import (
	"fmt"
	"strings"
	"sync"

	botgolang "github.com/mail-ru-im/bot-golang"

	"github.com/z0rr0/mtbot/db"
)

const (
	// internalError is common intrnal error message
	internalError = "internal error"
)

var (
	// knownHandlers is a map of known handling functions.
	knownHandlers = map[string]func(Sender, *Package) error{
		"/get":   Get,
		"/set":   Set,
		"/start": Start,
		"/stop":  Stop,
	}
	// publicErrors is map of internal errors to public users' messages.
	publicErrors = map[error]string{
		db.ErrUnknownUser: "not started",
		db.ErrKnownUser:   "already started",
		db.ErrSetUser:     "oops, no params, use space separated integers",
	}
)

// Package contains parameters from bot.
type Package struct {
	ChatID string
	Text   string
	params string
}

// String is a string representation of Package.
func (p *Package) String() string {
	return fmt.Sprintf("[%s] %s", p.ChatID, p.Text)
}

// Sender is interface to send a command response.
type Sender interface {
	Send(err error, chatID, text string) error
	Get(p *Package) (string, error)
	Set(p *Package) error
	Start(p *Package) error
	Stop(p *Package) error
	Log(info bool, format string, v ...interface{})
}

// Settings is a serve settings.
type Settings struct {
	*db.Logger
	Storage *db.Storage
	Bot     *botgolang.Bot
	Workers int
}

// Send is a method to implement Sender interface.
// It sends an error or success reply.
func (st *Settings) Send(err error, chatID, text string) error {
	if err != nil {
		errMsg, ok := publicErrors[err]
		if ok {
			text = errMsg
		} else {
			st.Error.Printf("chat=%s, response='%s': %v", chatID, text, err)
			text = "ERROR: " + text
		}
	}
	message := st.Bot.NewTextMessage(chatID, text)
	return message.Send()
}

// Get is a method to implement Sender interface.
// It gets storage info by p Package.
func (st *Settings) Get(p *Package) (string, error) {
	return st.Storage.Get(p.ChatID)
}

// Set is a method to implement Sender interface.
// It updates storage info p Package.
func (st *Settings) Set(p *Package) error {
	return st.Storage.Set(p.ChatID, p.params)
}

// Start is a method to implement Sender interface.
// It does storage start call.
func (st *Settings) Start(p *Package) error {
	return st.Storage.Start(p.ChatID)
}

// Stop is a method to implement Sender interface.
// It removes info from the storage.
func (st *Settings) Stop(p *Package) error {
	return st.Storage.Stop(p.ChatID)
}

// Log is a method to implement Sender interface.
// It does debug or error output.
func (st *Settings) Log(info bool, format string, v ...interface{}) {
	if info {
		st.Info.Printf(format, v...)
	} else {
		st.Error.Printf(format, v...)
	}
}

// SendError sends err as a bot response.
func (st *Settings) SendError(chatID string, err error) error {
	response := fmt.Sprintf("ERROR: %s", err.Error())
	message := st.Bot.NewTextMessage(chatID, response)
	return message.Send()
}

// Get is a handler when user gets its notifications.
func Get(s Sender, p *Package) error {
	response, err := s.Get(p)
	if err != nil {
		s.Log(false, "get error: %v", err)
		return s.Send(err, p.ChatID, internalError)
	}
	return s.Send(nil, p.ChatID, response)
}

// Set is a handler when user sends notifications scheduler.
func Set(s Sender, p *Package) error {
	err := s.Set(p)
	if err != nil {
		s.Log(false, "set error: %v", err)
		return s.Send(err, p.ChatID, internalError)
	}
	return s.Send(nil, p.ChatID, "OK")
}

// Start is a handler for new user adding.
func Start(s Sender, p *Package) error {
	err := s.Start(p)
	if err != nil {
		s.Log(false, "start error: %v", err)
		return s.Send(err, p.ChatID, internalError)
	}
	return s.Send(nil, p.ChatID, "started")
}

// Stop is a handler user removing.
func Stop(s Sender, p *Package) error {
	err := s.Stop(p)
	if err != nil {
		s.Log(false, "stop error: %v", err)
		return s.Send(err, p.ChatID, internalError)
	}
	return s.Send(nil, p.ChatID, "stopped")
}

// filter checks s is valid command value.
// It returns command and its parameters.
func filter(s string) (string, string) {
	s = strings.Trim(s, " ")
	if !strings.HasPrefix(s, "/") {
		return "", ""
	}
	values := strings.SplitN(s, " ", 2)
	switch len(values) {
	case 2:
		return values[0], values[1]
	case 1:
		return values[0], ""
	}
	return "", ""
}

// handle validates input string command and runs the handler.
func handle(st *Settings, p Package) error {
	c, v := filter(p.Text)
	if c == "" {
		st.Info.Printf("not command [%s]: %s", p.ChatID, p.Text)
		return nil
	}
	f, ok := knownHandlers[c]
	if !ok {
		st.Info.Printf(" unknown command [%s]: %s", p.ChatID, c)
		return nil
	}
	p.params = v
	return f(st, &p)
}

// Serve runs command handling workers.
// To initiate stop of handlers a closing of "commands" should be used.
// A returned waitGroup can be used to wait of handlers graceful stopping.
func Serve(st Settings, commands <-chan Package) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(st.Workers)
	for i := 0; i < st.Workers; i++ {
		go func(j int) {
			for p := range commands {
				st.Info.Printf("cmd worker=%d got p=%s", j, p.String())
				if err := handle(&st, p); err != nil {
					st.Error.Printf("failed handler command '%s', worker=%d: %v", p.String(), j, err)
				} else {
					st.Debug.Printf("worker=%d done", j)
				}
			}
			wg.Done()
		}(i)
	}
	return &wg
}
