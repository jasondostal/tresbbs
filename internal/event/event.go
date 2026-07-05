// Package event implements the scheduled event system for tresbbs.
//
// The original TriBBS had an event system that could run batch files at
// specific times on specific days. Events could "slide" — meaning they'd
// run at the next available opportunity after their scheduled time, rather
// than interrupting a user session.
//
// Event configuration (from TRIMAN.EXE):
//   - Time: "HH:MM" format
//   - Day: "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun", or "Daily"
//   - File: batch file to execute
//   - Slide: if true, run at next opportunity; if false, run at exact time
//
// Events are checked on each tick of the main loop. If an event's time has
// passed and it hasn't executed today, it's executed (or queued if sliding).
package event

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/port"
)

// Scheduler manages scheduled events.
type Scheduler struct {
	storage     port.StoragePort
	running     bool
	lastDay     string // Track the last day we checked for daily reset
}

// NewScheduler creates a new event scheduler.
func NewScheduler(storage port.StoragePort) *Scheduler {
	return &Scheduler{
		storage: storage,
		lastDay: time.Now().Format("2006-01-02"),
	}
}

// CheckAndExecute checks if any events need to run and executes them.
// This should be called on each tick of the main loop.
func (s *Scheduler) CheckAndExecute() error {
	now := time.Now()
	currentDay := now.Format("2006-01-02")
	
	// Check if we need to reset daily flags (new day)
	if currentDay != s.lastDay {
		log.Printf("New day detected (%s), resetting event flags", currentDay)
		if err := s.ResetDailyFlags(); err != nil {
			log.Printf("Error resetting daily flags: %v", err)
		}
		s.lastDay = currentDay
	}
	
	events, err := s.storage.GetEvents()
	if err != nil {
		return fmt.Errorf("loading events: %w", err)
	}

	currentDayName := now.Format("Mon") // "Mon", "Tue", etc.
	currentTime := now.Format("15:04")

	for _, event := range events {
		// Skip if already executed today
		if event.ExecutedToday {
			continue
		}

		// Check if this event should run today
		if !s.shouldRunToday(event, currentDayName) {
			continue
		}

		// Check if it's time to run
		if s.isTimeToRun(event, currentTime) {
			if event.Slide {
				// Sliding event — run at next opportunity
				// (when no users are online, or immediately for simplicity)
				if err := s.executeEvent(event); err != nil {
					log.Printf("Event execution failed: %v", err)
				}
			} else {
				// Non-sliding — run at exact time
				if err := s.executeEvent(event); err != nil {
					log.Printf("Event execution failed: %v", err)
				}
			}
		}
	}

	return nil
}

// ResetDailyFlags resets the executed_today flags for all events.
// This should be called at midnight.
func (s *Scheduler) ResetDailyFlags() error {
	events, err := s.storage.GetEvents()
	if err != nil {
		return err
	}

	for _, event := range events {
		event.ExecutedToday = false
		if err := s.storage.SaveEvent(&event); err != nil {
			log.Printf("Error resetting event flag: %v", err)
		}
	}

	log.Printf("Daily event flags reset for %d events", len(events))
	return nil
}

// GetPendingEvents returns events that are scheduled but haven't executed today.
func (s *Scheduler) GetPendingEvents() ([]domain.Event, error) {
	events, err := s.storage.GetEvents()
	if err != nil {
		return nil, err
	}
	
	var pending []domain.Event
	currentDayName := time.Now().Format("Mon")
	
	for _, event := range events {
		if !event.ExecutedToday && s.shouldRunToday(event, currentDayName) {
			pending = append(pending, event)
		}
	}
	
	return pending, nil
}

// shouldRunToday checks if an event should run on the given day.
func (s *Scheduler) shouldRunToday(event domain.Event, currentDay string) bool {
	if event.Day == "Daily" {
		return true
	}

	// Check for specific day match
	// The day field can be "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"
	// or comma-separated like "Mon,Wed,Fri"
	days := strings.Split(event.Day, ",")
	for _, d := range days {
		if strings.TrimSpace(d) == currentDay {
			return true
		}
	}

	return false
}

// isTimeToRun checks if it's time to run an event.
func (s *Scheduler) isTimeToRun(event domain.Event, currentTime string) bool {
	// Parse event time (HH:MM)
	eventTime := event.Time

	// For sliding events, run if current time is >= event time
	if event.Slide {
		return currentTime >= eventTime
	}

	// For non-sliding events, run at exact time (within 1-minute window)
	return currentTime == eventTime
}

// executeEvent runs an event's batch file.
func (s *Scheduler) executeEvent(event domain.Event) error {
	log.Printf("Executing event: %s (%s)", event.File, event.Time)

	// Execute the batch file
	cmd := exec.Command("sh", "-c", event.File)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Event %s output: %s", event.File, string(output))
		return fmt.Errorf("executing %s: %w", event.File, err)
	}

	// Mark as executed
	event.ExecutedToday = true
	if err := s.storage.SaveEvent(&event); err != nil {
		log.Printf("Error marking event as executed: %v", err)
	}

	log.Printf("Event %s completed successfully", event.File)
	return nil
}
