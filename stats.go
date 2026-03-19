package main

import "sync/atomic"

// RunStats accumulates counters across all pipeline steps for a single run.
// All fields are updated via atomic operations so they are safe if steps are
// ever parallelised in the future.
type RunStats struct {
	MovedFolders       atomic.Int64
	MovedFiles         atomic.Int64
	DuplicateFiles     atomic.Int64
	ConflictFiles      atomic.Int64
	NonAudioMoved      atomic.Int64
	FailedImportsSwept atomic.Int64
	Errors             atomic.Int64
}

func (s *RunStats) incMoved()          { s.MovedFolders.Add(1) }
func (s *RunStats) incMovedFile()      { s.MovedFiles.Add(1) }
func (s *RunStats) incDuplicate()      { s.DuplicateFiles.Add(1) }
func (s *RunStats) incConflict()       { s.ConflictFiles.Add(1) }
func (s *RunStats) incNonAudio()       { s.NonAudioMoved.Add(1) }
func (s *RunStats) incSwept()          { s.FailedImportsSwept.Add(1) }
func (s *RunStats) incError()          { s.Errors.Add(1) }
func (s *RunStats) anyFilesMoved() bool { return s.MovedFiles.Load() > 0 }
