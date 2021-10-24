package db

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	botgolang "github.com/mail-ru-im/bot-golang"
)

var (
	// ErrUnknownUser is an error when a request was gotten from unknown user.
	ErrUnknownUser = errors.New("unknown user")
	// ErrKnownUser is an error when the user already exists in the storage.
	ErrKnownUser = errors.New("known user")
	// ErrSetUser is error when set method was called with failed arguments.
	ErrSetUser = errors.New("no params")
)

// Limits stores users' limits.
type Limits struct {
	Users    int `toml:"users"`
	Delays   int `toml:"delays"`
	MinDelay int `toml:"min_delay"`
	MaxDelay int `toml:"max_delay"`
}

// Logger is common struct for loggers by levels.
type Logger struct {
	Debug *log.Logger
	Info  *log.Logger
	Error *log.Logger
}

// NewLogger returns new logger struct.
func NewLogger(debug bool) *Logger {
	logger := new(Logger)
	logger.Error = log.New(os.Stderr, "ERROR ", log.Ldate|log.Ltime|log.Lshortfile)
	logger.Info = log.New(os.Stdout, "INFO  ", log.LstdFlags)
	if debug {
		logger.Debug = log.New(os.Stdout, "DEBUG ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	} else {
		logger.Debug = log.New(ioutil.Discard, "DEBUG ", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	}
	return logger
}

// Event is a notification event's settings.
type Event struct {
	Title     string       `toml:"title"`
	URL       string       `toml:"url"`
	Message   string       `toml:"message"`
	Weekday   time.Weekday `toml:"weekday"`
	Period    string       `toml:"period"`
	StartHour string       `toml:"time"`
	TimeZone  string       `toml:"timezone"`
	offset    time.Duration
	alarm     time.Time // next event datetime
}

func (e *Event) validate() (*time.Location, time.Duration, error) {
	const dayHours = time.Hour * 24
	location, err := time.LoadLocation(e.TimeZone)
	if err != nil {
		return nil, 0, fmt.Errorf("parse zone=%s of event=%s: %w", e.TimeZone, e.Title, err)
	}
	offset, err := time.ParseDuration(e.Period)
	if err != nil {
		return nil, 0, fmt.Errorf("parse event=%s: %w", e.Title, err)
	}
	e.offset = offset

	startOffset, err := time.ParseDuration(e.StartHour)
	if err != nil {
		return nil, 0, fmt.Errorf("parse time of event=%s: %w", e.Title, err)
	}
	if (startOffset < 0) || (startOffset > dayHours) {
		return nil, 0, fmt.Errorf("invalid time of event=%s: %v", e.Title, startOffset)
	}
	return location, startOffset, nil
}

// Init validates event's parameters and sets internal time fields.
func (e *Event) Init() error {
	location, startOffset, err := e.validate()
	if err != nil {
		return err
	}
	now := time.Now().UTC().In(location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	alarmTime := today.Add(startOffset)

	w := alarmTime.Weekday()
	addDays := int(e.Weekday - w)
	e.alarm = nextAlarm(alarmTime.AddDate(0, 0, addDays), now, e.offset)
	return nil
}

// String is a string representation of the event info.
func (e *Event) String() string {
	return fmt.Sprintf(
		"Event [%s], ofsset=%v, alarm=%d:%.02d (%v)",
		e.Title, e.offset, e.alarm.Hour(), e.alarm.Minute(), e.alarm,
	)
}

// text returns full notification string message.
func (e *Event) text() string {
	return fmt.Sprintf("%s\n\n%s", e.Title, e.Message)
}

// userMsg is a struct for user event message.
type userMsg struct {
	user  string
	text  string
	url   string
	start string
	bot   *botgolang.Bot
}

// Send prepares and sends notification to the user.
func (m *userMsg) Send() error {
	message := m.bot.NewTextMessage(m.user, m.text)
	btn := botgolang.NewURLButton("URL", m.url)

	keyboard := botgolang.NewKeyboard()
	keyboard.AddRow(btn)
	message.AttachInlineKeyboard(keyboard)
	return message.Send()
}

// userEvent is user's alarm record.
type userEvent struct {
	user        string
	event       *Event
	delay       int
	delayOffset time.Duration
	timestamp   time.Time
}

// String is a string representation of user's event.
func (ue *userEvent) String() string {
	return ue.timestamp.Format(time.RFC3339)
}

// Message returns prepared user's event message.
func (ue *userEvent) Message(b *botgolang.Bot) userMsg {
	return userMsg{
		user:  ue.user,
		text:  ue.event.text(),
		url:   ue.event.URL,
		start: ue.timestamp.Add(ue.delayOffset).Format(time.RFC3339),
		bot:   b,
	}
}

// user is a client info struct.
type user struct {
	name   string
	delays []int
}

// stringDelays returns space-separated user's details as a string.
func (u *user) stringDelays() string {
	delays := make([]string, len(u.delays))
	for i, d := range u.delays {
		delays[i] = strconv.Itoa(d)
	}
	return strings.Join(delays, " ")
}

// init prepares user's event items.
func (u *user) init(events []*Event) []*userEvent {
	items := make([]*userEvent, 0, len(events)*len(u.delays))
	now := time.Now()
	for j, e := range events {
		na := nextAlarm(e.alarm, now, e.offset)
		for _, d := range u.delays {
			offset := time.Duration(d) * time.Minute
			i := &userEvent{
				user:        u.name,
				event:       events[j],
				delay:       d,
				delayOffset: offset,
				timestamp:   na.Add(-offset),
			}
			items = append(items, i)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].timestamp.Before(items[j].timestamp)
	})
	return items
}

// Storage is a main data storage struct.
type Storage struct {
	sync.RWMutex
	events    []*Event
	items     []*userEvent // items sorted by timestamp
	limits    Limits
	users     map[string]*user
	usersFile string                  // user log file
	userIdx   map[string][]*userEvent // user's items index
}

// New reads usersSource file, combines them with events and creates a new Storage object.
func New(usersSource string, events []*Event, l Limits) (*Storage, error) {
	users, usersFile, err := loadUsers(usersSource)
	if err != nil {
		return nil, err
	}
	s := &Storage{events: events, usersFile: usersFile, limits: l}
	s.init(users)
	return s, nil
}

// init builds base storage's structures.
func (s *Storage) init(users []*user) {
	s.Lock()
	n := len(users)
	s.users = make(map[string]*user, n)
	s.userIdx = make(map[string][]*userEvent, n)
	s.items = make([]*userEvent, 0, n) // n is only minimal hint
	for i, u := range users {
		items := users[i].init(s.events)
		s.users[u.name] = users[i]
		s.userIdx[u.name] = items
		s.items = append(s.items, items...)
	}
	sort.Slice(s.items, func(i, j int) bool {
		return s.items[i].timestamp.Before(s.items[j].timestamp)
	})
	s.Unlock()
}

// Start creates new user's notifications scheduler.
func (s *Storage) Start(userName string) error {
	s.Lock()
	defer s.Unlock()

	if n := len(s.users); n >= s.limits.Users {
		return fmt.Errorf("too many users %d > %d", n, s.limits.Users)
	}
	if _, ok := s.users[userName]; ok {
		// already know user
		return ErrKnownUser
	}
	u := &user{name: userName}
	s.users[userName] = u
	s.userIdx[userName] = make([]*userEvent, 0)
	// no new s.items for new user
	err := s.flush()
	if err != nil {
		return fmt.Errorf("start user=%s: %w", userName, err)
	}
	return nil
}

// Stop removes user from the storage.
func (s *Storage) Stop(userName string) error {
	s.Lock()
	defer s.Unlock()

	_, ok := s.users[userName]
	if !ok {
		return ErrUnknownUser
	}
	delete(s.users, userName)
	delete(s.userIdx, userName)

	storageItems := make([]*userEvent, 0, len(s.items))
	for j, i := range s.items {
		if i.user != userName {
			// save other users' items
			storageItems = append(storageItems, s.items[j])
		}
	}
	s.items = storageItems
	sort.Slice(s.items, func(i, j int) bool {
		return s.items[i].timestamp.Before(s.items[j].timestamp)
	})
	err := s.flush()
	if err != nil {
		return fmt.Errorf("stop user=%s: %w", userName, err)
	}
	return nil
}

// Get returns user's delays.
func (s *Storage) Get(userName string) (string, error) {
	s.RLock()
	defer s.RUnlock()

	u, ok := s.users[userName]
	if !ok {
		return "", ErrUnknownUser
	}
	if len(u.delays) == 0 {
		return "You have not notifications", nil
	}
	result := fmt.Sprintf("Your parameters: %s\n\nNotifications:", u.stringDelays())
	for _, ue := range s.userIdx[userName] {
		result += fmt.Sprintf("\n%s", ue.String())
	}
	return result, nil
}

// Set changes user's delay values
func (s *Storage) Set(userName, values string) error {
	if values == "" {
		return ErrSetUser
	}
	s.Lock()
	defer s.Unlock()

	u, ok := s.users[userName]
	if !ok {
		return ErrUnknownUser
	}
	_, delays, err := parseUserRow([]string{userName, values}, s.limits.MaxDelay, s.limits.MinDelay, s.limits.Delays)
	if err != nil {
		return fmt.Errorf("set user: %w", err)
	}
	u.delays = delays
	items := u.init(s.events)
	s.users[u.name] = u
	s.userIdx[u.name] = items
	// save persistent data
	if err = s.flush(); err != nil {
		return fmt.Errorf("save updated user=%s: %w", userName, err)
	}
	storageItems := make([]*userEvent, 0, len(s.items))
	for j, i := range s.items {
		if i.user != u.name {
			// save other users' items
			storageItems = append(storageItems, s.items[j])
		}
	}
	storageItems = append(storageItems, items...)
	s.items = storageItems
	sort.Slice(s.items, func(i, j int) bool {
		return s.items[i].timestamp.Before(s.items[j].timestamp)
	})
	return nil
}

// Close does operations to safety save any data.
func (s *Storage) Close() error {
	s.Lock()
	defer s.Unlock()
	return s.flush()
}

// notifications checks new applied users' messages.
func (s *Storage) notifications(b *botgolang.Bot) []userMsg {
	var (
		now           = time.Now()
		notifications = make([]userMsg, 0)
	)
	s.Lock()
	defer s.Unlock()

	for j := range s.items {
		i := s.items[j]
		if i.timestamp.Before(now) {
			notifications = append(notifications, i.Message(b))
			i.timestamp = i.timestamp.Add(i.event.offset)
		} else {
			break
		}
	}
	sort.Slice(s.items, func(i, j int) bool {
		return s.items[i].timestamp.Before(s.items[j].timestamp)
	})
	return notifications
}

// flush rewrites users CSV file. The caller should use storage locking.
func (s *Storage) flush() error {
	f, err := os.OpenFile(s.usersFile, os.O_WRONLY|os.O_TRUNC, 0660)
	if err != nil {
		return fmt.Errorf("users log open to save: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()
	rows := make([][]string, 0, len(s.users))
	for userName, u := range s.users {
		rows = append(rows, []string{userName, u.stringDelays()})
	}
	// sort by username
	sort.Slice(rows, func(i, j int) bool {
		return rows[i][0] < rows[j][0]
	})
	w := csv.NewWriter(f)
	if err = w.WriteAll(rows); err != nil {
		return fmt.Errorf("users log write: %w", err)
	}
	w.Flush()
	if err = w.Error(); err != nil {
		return fmt.Errorf("users log flush: %w", err)
	}
	return nil
}

// Show prints items info using logger l.
func (s *Storage) Show(l *log.Logger) {
	l.Println("show items info")
	for i, x := range s.items {
		l.Printf(
			"[%d]: user=%s, delay=%d, event=%v, alarm=%v\n",
			i, x.user, x.delay, x.event.Title, x.timestamp,
		)
	}
}

func parseUserRow(userItem []string, minD, maxD, maxDelays int) (string, []int, error) {
	const userValues = 2
	if n := len(userItem); n != userValues {
		return "", nil, fmt.Errorf("failed parse user data, len=%d: %v", n, userItem)
	}
	strDelays := strings.Split(strings.Trim(userItem[1], " "), " ")
	uniqDelays := make(map[int]struct{}, len(strDelays))
	for _, d := range strDelays {
		j, err := strconv.Atoi(d)
		if err != nil {
			return "", nil, fmt.Errorf("failed parse user delays=%s, %v: %w", d, userItem, err)
		}
		if (minD > 0) && (j < minD) {
			return "", nil, fmt.Errorf("too small delay %d < %d", j, minD)
		}
		if (maxD > 0) && (j > maxD) {
			return "", nil, fmt.Errorf("too large delay %d > %d", j, maxD)
		}
		uniqDelays[j] = struct{}{}
	}
	lenDelays := len(uniqDelays)
	if (maxDelays > 0) && (lenDelays > maxDelays) {
		return "", nil, fmt.Errorf("too many user's delays %d > %d", lenDelays, maxDelays)
	}
	delays := make([]int, 0, lenDelays)
	for d := range uniqDelays {
		delays = append(delays, d)
	}
	sort.Ints(delays)
	return strings.Trim(userItem[0], " "), delays, nil
}

// loadUsers loads users' names and delays form a source CSV file.
func loadUsers(usersFile string) ([]*user, string, error) {
	fullPath, err := filepath.Abs(strings.Trim(usersFile, " "))
	if err != nil {
		return nil, "", fmt.Errorf("users log file: %w", err)
	}
	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_RDONLY, 0640)
	if err != nil {
		return nil, "", fmt.Errorf("users log open: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, "", fmt.Errorf("users log parse: %w", err)
	}
	userRecords := make([]*user, 0, len(records))
	for _, userItem := range records {
		// ignore delay limit during reading file data
		name, delays, err := parseUserRow(userItem, 0, 0, 0)
		if err != nil {
			return nil, "", fmt.Errorf("users row parse: %w", err)
		}
		userRecords = append(userRecords, &user{name: name, delays: delays})
	}
	return userRecords, fullPath, nil
}

// nextAlarm returns next alarm time after dt, offset is a repeatable alarm's period.
func nextAlarm(alarm, dt time.Time, offset time.Duration) time.Time {
	if alarm.After(dt) {
		return alarm
	}
	diff := dt.Sub(alarm)
	periods := diff / offset
	if diff%offset > 0 {
		periods++
	}
	next := alarm.Add(offset * periods)
	// for spring/autumn offset change
	_, offsetBefore := alarm.Zone()
	_, offsetAfter := next.Zone()
	return next.Add(time.Second * time.Duration(offsetBefore-offsetAfter))
}

// Settings is a serve settings.
type Settings struct {
	*Logger
	TickPeriod time.Duration
	Workers    int
	Bot        *botgolang.Bot
}

// Serve runs users' notifications handling monitoring.
func Serve(ctx context.Context, s *Storage, st Settings) *sync.WaitGroup {
	var (
		wg       sync.WaitGroup
		notifier = make(chan userMsg)
	)
	go func() {
		ticker := time.NewTicker(st.TickPeriod)
		defer func() {
			ticker.Stop()
			close(notifier)
		}()
		for {
			select {
			case <-ctx.Done():
				st.Info.Println("db serve ctx done")
				return
			case <-ticker.C:
				items := s.notifications(st.Bot)
				st.Info.Printf("found for notifications %d items", len(items))
				for i := range items {
					notifier <- items[i]
				}
			}
		}
	}()
	wg.Add(st.Workers)
	for i := 0; i < st.Workers; i++ {
		go func(j int) {
			for m := range notifier {
				st.Debug.Printf("handle notification [worker=%d]: %v", j, m.user)
				if err := m.Send(); err != nil {
					st.Error.Printf("failed send message worker=%d [%v]: %v", j, m, err)
				}
			}
			wg.Done()
		}(i)
	}
	return &wg
}
