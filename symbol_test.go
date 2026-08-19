package oida

import (
	"net/http"
	"testing"
)

type userStorage struct{}

func (userStorage) Get() {}

func TestSymbolName(t *testing.T) {
	storage := userStorage{}

	cases := []struct {
		name  string
		input any
		want  string
	}{
		{"value names its type", storage, "oida.userStorage"},
		{"method value names the method", storage.Get, "oida.userStorage.Get"},
		{"package function keeps its package", http.NewRequest, "http.NewRequest"},
		{"interface method names the concrete type", http.DefaultClient.Get, "http.Client.Get"},
		{"exported function of this package", RecordError, "oida.RecordError"},
		{"builtin type", 32, "int"},
		{"a string passes through", "user.service.Login", "user.service.Login"},
		{"a path keeps its last element", "github.com/titpetric/oida/user.Login", "user.Login"},
		{"nil is legal but useless", nil, "<nil>"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := symbolName(testCase.input); got != testCase.want {
				t.Fatalf("symbolName(%v) = %q, want %q", testCase.input, got, testCase.want)
			}
		})
	}
}
