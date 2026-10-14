package main

import (
	"context"
	"errors"
	"github.com/cy4268/momiao/internal/platform"
	"testing"
)

type registrationWorkerFake struct {
	cursor                        int64
	ingests, failures, recoveries int
	failCommit                    bool
	readErr                       error
	reads                         []int64
}

func (s *registrationWorkerFake) ReadRegistrationPage(_ context.Context, after int64, _ int) (platform.RegistrationPage, error) {
	s.reads = append(s.reads, after)
	return platform.RegistrationPage{NextCursor: after + 1}, s.readErr
}

func (s *registrationWorkerFake) RegistrationCursor(context.Context) (int64, error) {
	return s.cursor, nil
}
func (s *registrationWorkerFake) MarkRegistrationSourceUnavailable(context.Context) error {
	s.failures++
	return nil
}
func (s *registrationWorkerFake) IngestRegistrationPage(_ context.Context, _ int64, p platform.RegistrationPage) error {
	s.ingests++
	if s.failCommit {
		return errors.New("synthetic unknown commit")
	}
	s.cursor = p.NextCursor
	return nil
}
func (s *registrationWorkerFake) RecoverRegistrationGrant(context.Context) (bool, error) {
	s.recoveries++
	return false, nil
}
func TestAdmissionWorkerSourceFailureStillRecoversJobs(t *testing.T) {
	s := &registrationWorkerFake{cursor: 7, readErr: errors.New("local receipt read failed")}
	registrationWorkerCycle(context.Background(), s)
	if s.cursor != 7 || s.ingests != 0 || s.failures != 1 || s.recoveries != 1 {
		t.Fatal("source failure lost cursor or stopped existing recovery")
	}
}
func TestAdmissionWorkerFailedCommitReusesOriginalCursor(t *testing.T) {
	s := &registrationWorkerFake{failCommit: true}
	registrationWorkerCycle(context.Background(), s)
	s.failCommit = false
	registrationWorkerCycle(context.Background(), s)
	if s.cursor != 1 || s.ingests != 2 || s.recoveries != 2 || len(s.reads) != 2 || s.reads[0] != 0 || s.reads[1] != 0 {
		t.Fatal("original page not recovered")
	}
}
