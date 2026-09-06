package main

import (
	"context"
	"errors"
	"github.com/cy4268/momiao/internal/platform"
	"net/http"
	"strings"
	"testing"
)

type registrationWorkerFake struct {
	cursor                        int64
	ingests, failures, recoveries int
	failCommit                    bool
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
	s := &registrationWorkerFake{cursor: 7}
	registrationWorkerCycle(context.Background(), s, admissionTransport(func(*http.Request) (*http.Response, error) {
		return readerResponse(503, "private upstream detail"), nil
	}), strings.Repeat("k", 32))
	if s.cursor != 7 || s.ingests != 0 || s.failures != 1 || s.recoveries != 1 {
		t.Fatal("source failure lost cursor or stopped existing recovery")
	}
}
func TestAdmissionWorkerFailedCommitReusesOriginalCursor(t *testing.T) {
	s := &registrationWorkerFake{failCommit: true}
	f := admissionTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("after") != "0" {
			t.Fatal("cursor advanced before durable commit")
		}
		return readerResponse(200, `{"success":true,"data":{"receipts":[`+syntheticReceipt+`],"next_cursor":1}}`), nil
	})
	registrationWorkerCycle(context.Background(), s, f, strings.Repeat("k", 32))
	s.failCommit = false
	registrationWorkerCycle(context.Background(), s, f, strings.Repeat("k", 32))
	if s.cursor != 1 || s.ingests != 2 || s.recoveries != 2 {
		t.Fatal("original page not recovered")
	}
}
