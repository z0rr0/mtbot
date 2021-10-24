package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"

	botgolang "github.com/mail-ru-im/bot-golang"

	"github.com/z0rr0/mtbot/cmd"
	"github.com/z0rr0/mtbot/config"
	"github.com/z0rr0/mtbot/db"
)

const (
	// Name is a program name.
	Name = "MtBot"
	// Config is default configuration file name.
	Config = "config.toml"
)

var (
	// Version is git version
	Version = ""
	// Revision is revision number
	Revision = ""
	// BuildDate is build date
	BuildDate = ""
	// GoVersion is runtime Go language version
	GoVersion = runtime.Version()
	// allowedBotEvents are bot events for handling
	allowedBotEvents = map[botgolang.EventType]bool{
		botgolang.NEW_MESSAGE:    true,
		botgolang.EDITED_MESSAGE: true,
	}
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintf(os.Stderr, "abnormal termination [%v]: %v\n%v", Version, r, string(debug.Stack()))
		}
	}()
	version := flag.Bool("version", false, "show version")
	cfg := flag.String("config", Config, "configuration file")
	flag.Parse()

	if *version {
		fmt.Printf("%v: %v %v %v %v\n", Name, Version, Revision, GoVersion, BuildDate)
		flag.PrintDefaults()
		return
	}
	c, err := config.New(*cfg)
	if err != nil {
		panic(err)
	}
	for i, e := range c.Events {
		c.Debug.Printf("e [%d] = %v", i, e)
	}

	c.Debug.Println("build new db")
	s, err := db.New(c.M.Database, c.Events, c.L)
	if err != nil {
		panic(err)
	}
	s.Show(c.Debug)

	ctx, cancel := context.WithCancel(context.Background())
	stDB := db.Settings{TickPeriod: c.Period, Workers: c.W.Notify, Logger: c.Logger, Bot: c.B}
	wgDB := db.Serve(ctx, s, stDB)

	commands := make(chan cmd.Package)
	stCmd := cmd.Settings{Storage: s, Bot: c.B, Workers: c.W.User, Logger: c.Logger}
	wgCmd := cmd.Serve(stCmd, commands)

	go serve(ctx, cancel, c, commands)

	wgDB.Wait()  // wait periodic notifications stopping
	wgCmd.Wait() // wait user command handling stopping
	if err = s.Close(); err != nil {
		c.Error.Printf("failed close storage: %v", err)
	}
	c.Info.Printf("stopped %s", Name)
}

func serve(ctx context.Context, cancel context.CancelFunc, c *config.Config, commands chan<- cmd.Package) {
	var (
		sigint = make(chan os.Signal, 1)
		events = c.B.GetUpdatesChannel(ctx)
	)
	defer func() {
		close(sigint)
		close(commands)
		cancel()
	}()
	signal.Notify(sigint, os.Interrupt, os.Signal(syscall.SIGTERM), os.Signal(syscall.SIGQUIT))
	for {
		select {
		case s := <-sigint:
			c.Info.Printf("taken signal %v", s)
			return
		case e := <-events:
			if allowedBotEvents[e.Type] {
				message := e.Payload.Message()
				if strings.HasPrefix(message.Text, "/") {
					c.Debug.Printf("gotten event type=%v from %s", e.Type, message.Chat.ID)
					commands <- cmd.Package{ChatID: message.Chat.ID, Text: message.Text}
				}
			}
		}
	}
}
