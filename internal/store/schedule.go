package store

import (
	"context"
	"errors"
	"time"

	"chunsu/internal/files"
)

const QueuePausedSetting = "queue_paused"
const MinimumScheduleInterval = time.Second

type Schedule struct {
	ID           string `json:"id"`
	ConnectionID string `json:"connection_id"`
	Name         string `json:"name"`
	IntervalMS   int64  `json:"interval_ms"`
	Timezone     string `json:"timezone"`
	NextRunAt    int64  `json:"next_run_at"`
	Enabled      bool   `json:"enabled"`
	LastTickID   string `json:"last_tick_id"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}
type Tick struct {
	ID                 string `json:"id"`
	ScheduleID         string `json:"schedule_id"`
	ScheduledFor       int64  `json:"scheduled_for"`
	TriggeredAt        int64  `json:"triggered_at"`
	MissedIntervals    int64  `json:"missed_intervals"`
	AcquisitionID      string `json:"acquisition_id"`
	ContinueID         string `json:"continue_id"`
	JobID              string `json:"job_id"`
	Status             string `json:"status"`
	NotBefore          int64  `json:"not_before"`
	Diagnostic         string `json:"diagnostic"`
	CollectionAttempts int    `json:"collection_attempts"`
}

const scheduleColumns = "id,connection_id,name,interval_ms,timezone,next_run_at,enabled,last_tick_id,created_at,updated_at"
const tickColumns = "id,schedule_id,scheduled_for,triggered_at,missed_intervals,acquisition_id,continue_id,job_id,status,not_before,diagnostic,collection_attempts"

func scanSchedule(row scanner) (Schedule, error) {
	var v Schedule
	err := row.Scan(&v.ID, &v.ConnectionID, &v.Name, &v.IntervalMS, &v.Timezone, &v.NextRunAt, &v.Enabled, &v.LastTickID, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}
func scanTick(row scanner) (Tick, error) {
	var v Tick
	err := row.Scan(&v.ID, &v.ScheduleID, &v.ScheduledFor, &v.TriggeredAt, &v.MissedIntervals, &v.AcquisitionID, &v.ContinueID, &v.JobID, &v.Status, &v.NotBefore, &v.Diagnostic, &v.CollectionAttempts)
	return v, err
}
func (s *Store) AddSchedule(ctx context.Context, connection, name, zone string, interval time.Duration) (Schedule, error) {
	var schedule Schedule
	if !files.ValidID(connection) || name == "" || interval < MinimumScheduleInterval {
		return schedule, errors.New("schedule needs a connection, name and interval of at least one second")
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return schedule, err
	}
	schedule = Schedule{ID: files.ID(), ConnectionID: connection, Name: name, IntervalMS: interval.Milliseconds(), Timezone: zone, NextRunAt: time.Now().Add(interval).UnixMilli(), CreatedAt: now(), UpdatedAt: now()}
	_, err := s.DB.ExecContext(ctx, "INSERT INTO schedules("+scheduleColumns+") VALUES(?,?,?,?,?,?,?,?,?,?)", schedule.ID, connection, name, schedule.IntervalMS, zone, schedule.NextRunAt, false, "", schedule.CreatedAt, schedule.UpdatedAt)
	return schedule, err
}
func (s *Store) Schedule(ctx context.Context, id string) (Schedule, error) {
	return scanSchedule(s.DB.QueryRowContext(ctx, "SELECT "+scheduleColumns+" FROM schedules WHERE id=?", id))
}
func (s *Store) Schedules(ctx context.Context) ([]Schedule, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+scheduleColumns+" FROM schedules ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Schedule{}
	for rows.Next() {
		v, e := scanSchedule(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) EnableSchedule(ctx context.Context, id string, enabled bool) error {
	result, err := s.DB.ExecContext(ctx, "UPDATE schedules SET enabled=?,updated_at=? WHERE id=?", enabled, now(), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return errors.New("schedule does not exist")
	}
	return err
}
func (s *Store) Tick(ctx context.Context, id string) (Tick, error) {
	return scanTick(s.DB.QueryRowContext(ctx, "SELECT "+tickColumns+" FROM schedule_ticks WHERE id=?", id))
}
func (s *Store) Ticks(ctx context.Context) ([]Tick, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+tickColumns+" FROM schedule_ticks ORDER BY triggered_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tick{}
	for rows.Next() {
		t, e := scanTick(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func NextInterval(schedule Schedule, at int64) (next, missed int64) {
	if at < schedule.NextRunAt {
		return schedule.NextRunAt, 0
	}
	steps := (at-schedule.NextRunAt)/schedule.IntervalMS + 1
	return schedule.NextRunAt + steps*schedule.IntervalMS, steps - 1
}
func (s *Store) BeginTick(ctx context.Context, id, continuation string) (Tick, error) {
	var tick Tick
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return tick, err
	}
	defer tx.Rollback()
	schedule, err := scanSchedule(tx.QueryRowContext(ctx, "SELECT "+scheduleColumns+" FROM schedules WHERE id=?", id))
	if err != nil {
		return tick, err
	}
	stamp := now()
	if !schedule.Enabled || schedule.NextRunAt > stamp {
		return tick, errors.New("schedule is not due")
	}
	if schedule.LastTickID != "" {
		previous, err := scanTick(tx.QueryRowContext(ctx, "SELECT "+tickColumns+" FROM schedule_ticks WHERE id=?", schedule.LastTickID))
		if err != nil {
			return tick, err
		}
		if previous.Status != "done" && previous.Status != "skipped" {
			return tick, errors.New("previous schedule occurrence still requires completion or intervention")
		}
	}
	next, missed := NextInterval(schedule, stamp)
	tick = Tick{ID: files.ID(), ScheduleID: id, ScheduledFor: schedule.NextRunAt, TriggeredAt: stamp, MissedIntervals: missed, AcquisitionID: files.ID(), ContinueID: continuation, Status: "pending"}
	if _, err = tx.ExecContext(ctx, "INSERT INTO schedule_ticks("+tickColumns+") VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", tick.ID, id, tick.ScheduledFor, tick.TriggeredAt, tick.MissedIntervals, tick.AcquisitionID, tick.ContinueID, "", tick.Status, 0, "", 0); err != nil {
		return tick, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE schedules SET next_run_at=?,last_tick_id=?,updated_at=? WHERE id=?", next, tick.ID, stamp, id); err != nil {
		return tick, err
	}
	return tick, tx.Commit()
}
func (s *Store) SaveTick(ctx context.Context, t Tick) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE schedule_ticks SET acquisition_id=?,job_id=?,status=?,not_before=?,diagnostic=?,collection_attempts=? WHERE id=?", t.AcquisitionID, t.JobID, t.Status, t.NotBefore, t.Diagnostic, t.CollectionAttempts, t.ID)
	return err
}
func (s *Store) SetPaused(ctx context.Context, paused bool) error {
	value := "false"
	if paused {
		value = "true"
	}
	_, err := s.DB.ExecContext(ctx, "UPDATE local_settings SET value=? WHERE key=?", value, QueuePausedSetting)
	return err
}
func (s *Store) Paused(ctx context.Context) (bool, error) {
	var v string
	err := s.DB.QueryRowContext(ctx, "SELECT value FROM local_settings WHERE key=?", QueuePausedSetting).Scan(&v)
	return v == "true", err
}
