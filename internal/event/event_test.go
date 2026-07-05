package event

import (
	"testing"
	"time"

	"github.com/jasondostal/tresbbs/domain"
)

// MockStoragePort implements a simple in-memory storage for testing.
type MockStoragePort struct {
	events []domain.Event
}

// Event operations
func (m *MockStoragePort) GetEvents() ([]domain.Event, error) {
	return m.events, nil
}

func (m *MockStoragePort) SaveEvent(event *domain.Event) error {
	for i, e := range m.events {
		if e.ID == event.ID {
			m.events[i] = *event
			return nil
		}
	}
	m.events = append(m.events, *event)
	return nil
}

func (m *MockStoragePort) DeleteEvent(id int) error {
	return nil
}

// Stub implementations for other required methods
func (m *MockStoragePort) GetUser(int) (*domain.User, error)                           { return nil, nil }
func (m *MockStoragePort) GetUserByName(string) (*domain.User, error)                  { return nil, nil }
func (m *MockStoragePort) GetUserByAlias(string) (*domain.User, error)                 { return nil, nil }
func (m *MockStoragePort) SaveUser(*domain.User) error                                 { return nil }
func (m *MockStoragePort) AddUser(*domain.User) (int, error)                           { return 0, nil }
func (m *MockStoragePort) DeleteUser(int) error                                        { return nil }
func (m *MockStoragePort) PackUsers() (int, error)                                     { return 0, nil }
func (m *MockStoragePort) ListUsers() ([]domain.User, error)                           { return nil, nil }
func (m *MockStoragePort) UserCount() (int, error)                                     { return 0, nil }
func (m *MockStoragePort) GetConferences() ([]domain.Conference, error)                 { return nil, nil }
func (m *MockStoragePort) GetConference(string) (*domain.Conference, error)             { return nil, nil }
func (m *MockStoragePort) AddConference(*domain.Conference) error                       { return nil }
func (m *MockStoragePort) SaveConference(*domain.Conference) error                      { return nil }
func (m *MockStoragePort) DeleteConference(string) error                               { return nil }
func (m *MockStoragePort) GetPosts(string, int) ([]domain.Post, error)                 { return nil, nil }
func (m *MockStoragePort) GetPost(int64) (*domain.Post, error)                         { return nil, nil }
func (m *MockStoragePort) AddPost(*domain.Post) error                                  { return nil }
func (m *MockStoragePort) DeletePost(int64) error                                      { return nil }
func (m *MockStoragePort) PostCount(string) (int64, error)                             { return 0, nil }
func (m *MockStoragePort) GetFileAreas() ([]domain.FileArea, error)                     { return nil, nil }
func (m *MockStoragePort) GetFileArea(string) (*domain.FileArea, error)                 { return nil, nil }
func (m *MockStoragePort) AddFileArea(*domain.FileArea) error                           { return nil }
func (m *MockStoragePort) SaveFileArea(*domain.FileArea) error                          { return nil }
func (m *MockStoragePort) DeleteFileArea(string) error                                 { return nil }
func (m *MockStoragePort) GetFiles(string, int) ([]domain.FileEntry, error)             { return nil, nil }
func (m *MockStoragePort) AddFile(*domain.FileEntry) error                             { return nil }
func (m *MockStoragePort) SaveFile(*domain.FileEntry) error                            { return nil }
func (m *MockStoragePort) DeleteFile(int64) error                                      { return nil }
func (m *MockStoragePort) GetBulletins() ([]domain.Bulletin, error)                     { return nil, nil }
func (m *MockStoragePort) GetBulletin(int) (*domain.Bulletin, error)                    { return nil, nil }
func (m *MockStoragePort) AddBulletin(*domain.Bulletin) error                           { return nil }
func (m *MockStoragePort) SaveBulletin(*domain.Bulletin) error                          { return nil }
func (m *MockStoragePort) DeleteBulletin(int) error                                    { return nil }
func (m *MockStoragePort) GetDoors() ([]domain.Door, error)                             { return nil, nil }
func (m *MockStoragePort) GetDoor(string) (*domain.Door, error)                         { return nil, nil }
func (m *MockStoragePort) AddDoor(*domain.Door) error                                  { return nil }
func (m *MockStoragePort) LogCaller(string) error                                      { return nil }
func (m *MockStoragePort) Close() error                                                 { return nil }
// Door operations
func (m *MockStoragePort) SaveDoor(*domain.Door) error                                   { return nil }
func (m *MockStoragePort) DeleteDoor(string) error                                       { return nil }
// Config operations
func (m *MockStoragePort) GetConfig() (*domain.Config, error)                            { return nil, nil }
func (m *MockStoragePort) SaveConfig(*domain.Config) error                               { return nil }
// Caller log
func (m *MockStoragePort) GetCallerLog(int) ([]string, error)                            { return nil, nil }

func TestShouldRunToday(t *testing.T) {
	scheduler := NewScheduler(&MockStoragePort{})

	tests := []struct {
		event     domain.Event
		day       string
		expected  bool
	}{
		{domain.Event{Day: "Daily"}, "Mon", true},
		{domain.Event{Day: "Daily"}, "Sun", true},
		{domain.Event{Day: "Mon"}, "Mon", true},
		{domain.Event{Day: "Mon"}, "Tue", false},
		{domain.Event{Day: "Mon,Wed,Fri"}, "Wed", true},
		{domain.Event{Day: "Mon,Wed,Fri"}, "Thu", false},
		{domain.Event{Day: "Sat,Sun"}, "Sat", true},
		{domain.Event{Day: "Sat,Sun"}, "Mon", false},
	}

	for _, tt := range tests {
		result := scheduler.shouldRunToday(tt.event, tt.day)
		if result != tt.expected {
			t.Errorf("shouldRunToday(%q, %q) = %v, want %v", tt.event.Day, tt.day, result, tt.expected)
		}
	}
}

func TestIsTimeToRun(t *testing.T) {
	scheduler := NewScheduler(&MockStoragePort{})

	tests := []struct {
		event     domain.Event
		current   string
		expected  bool
	}{
		// Non-sliding: exact time match
		{domain.Event{Time: "12:00", Slide: false}, "12:00", true},
		{domain.Event{Time: "12:00", Slide: false}, "12:01", false},
		{domain.Event{Time: "12:00", Slide: false}, "11:59", false},
		// Sliding: >= time
		{domain.Event{Time: "12:00", Slide: true}, "12:00", true},
		{domain.Event{Time: "12:00", Slide: true}, "12:01", true},
		{domain.Event{Time: "12:00", Slide: true}, "15:30", true},
		{domain.Event{Time: "12:00", Slide: true}, "11:59", false},
	}

	for _, tt := range tests {
		result := scheduler.isTimeToRun(tt.event, tt.current)
		if result != tt.expected {
			t.Errorf("isTimeToRun(%q@%q, slide=%v) = %v, want %v",
				tt.event.Time, tt.current, tt.event.Slide, result, tt.expected)
		}
	}
}

func TestResetDailyFlags(t *testing.T) {
	storage := &MockStoragePort{
		events: []domain.Event{
			{ID: 1, ExecutedToday: true},
			{ID: 2, ExecutedToday: true},
			{ID: 3, ExecutedToday: false},
		},
	}

	scheduler := NewScheduler(storage)
	err := scheduler.ResetDailyFlags()
	if err != nil {
		t.Fatalf("ResetDailyFlags() error: %v", err)
	}

	for _, event := range storage.events {
		if event.ExecutedToday {
			t.Errorf("Event %d should have ExecutedToday=false", event.ID)
		}
	}
}

func TestGetPendingEvents(t *testing.T) {
	storage := &MockStoragePort{
		events: []domain.Event{
			{ID: 1, Time: "00:00", Day: "Daily", ExecutedToday: true},  // Already executed
			{ID: 2, Time: "00:00", Day: "Daily", ExecutedToday: false}, // Pending
			{ID: 3, Time: "23:59", Day: "Daily", ExecutedToday: false}, // Future time
			{ID: 4, Time: "00:00", Day: "Mon", ExecutedToday: false},   // Different day
		},
	}

	scheduler := NewScheduler(storage)
	pending, err := scheduler.GetPendingEvents()
	if err != nil {
		t.Fatalf("GetPendingEvents() error: %v", err)
	}

	// Should have events 2 and 3 (past time, not executed) and maybe 4 (depends on day)
	if len(pending) < 2 {
		t.Errorf("Expected at least 2 pending events, got %d", len(pending))
	}
}

func BenchmarkCheckAndExecute(b *testing.B) {
	storage := &MockStoragePort{
		events: make([]domain.Event, 100),
	}

	// Create 100 events
	for i := 0; i < 100; i++ {
		storage.events[i] = domain.Event{
			ID:            i,
			Time:          "00:00",
			Day:           "Daily",
			ExecutedToday: true, // Already executed to avoid side effects
		}
	}

	scheduler := NewScheduler(storage)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scheduler.CheckAndExecute()
	}
}

// Helper function to get current day name
func getCurrentDay() string {
	return time.Now().Format("Mon")
}
