/*
Copyright 2026 The Protem Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"strings"
	"testing"
	"time"
)

type testFlags struct {
	fs        *flag.FlagSet
	hubName   string
	devAuth   bool
	retention time.Duration
}

func newTestFlags() *testFlags {
	f := &testFlags{fs: flag.NewFlagSet("test", flag.ContinueOnError)}
	f.fs.StringVar(&f.hubName, "hub-cluster-name", "local", "")
	f.fs.BoolVar(&f.devAuth, "dev-auth", false, "")
	f.fs.DurationVar(&f.retention, "history-retention", 0, "")
	return f
}

func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}

func TestFlagsFromEnvSetsValue(t *testing.T) {
	f := newTestFlags()
	env := map[string]string{"PROTEM_HUB_CLUSTER_NAME": "hub-1", "PROTEM_HISTORY_RETENTION": "2h"}
	if err := flagsFromEnv(f.fs, lookupFrom(env)); err != nil {
		t.Fatal(err)
	}
	if err := f.fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if f.hubName != "hub-1" || f.retention != 2*time.Hour {
		t.Fatalf("got hub-cluster-name=%q history-retention=%v, want hub-1 and 2h", f.hubName, f.retention)
	}
}

func TestFlagsFromEnvCommandLineWins(t *testing.T) {
	f := newTestFlags()
	if err := flagsFromEnv(f.fs, lookupFrom(map[string]string{"PROTEM_HUB_CLUSTER_NAME": "from-env"})); err != nil {
		t.Fatal(err)
	}
	if err := f.fs.Parse([]string{"--hub-cluster-name=from-args"}); err != nil {
		t.Fatal(err)
	}
	if f.hubName != "from-args" {
		t.Fatalf("hub-cluster-name = %q, want from-args", f.hubName)
	}
}

func TestFlagsFromEnvInvalidValueNamesVariable(t *testing.T) {
	for name, value := range map[string]string{"PROTEM_DEV_AUTH": "maybe", "PROTEM_HISTORY_RETENTION": "soon"} {
		err := flagsFromEnv(newTestFlags().fs, lookupFrom(map[string]string{name: value}))
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s=%s: error %v, want one naming %s", name, value, err, name)
		}
	}
}

func TestFlagsFromEnvIgnoresUnrelatedVariables(t *testing.T) {
	f := newTestFlags()
	env := map[string]string{"HUB_CLUSTER_NAME": "x", "PROTEM_UNKNOWN": "x", "PROTEM_DEV_AUTH_EXTRA": "maybe"}
	if err := flagsFromEnv(f.fs, lookupFrom(env)); err != nil {
		t.Fatal(err)
	}
	if f.hubName != "local" || f.devAuth {
		t.Fatalf("got hub-cluster-name=%q dev-auth=%v, want defaults", f.hubName, f.devAuth)
	}
}
