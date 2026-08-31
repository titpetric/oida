package internal

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
		{"value names its type", storage, "internal.userStorage"},
		{"method value names the method", storage.Get, "internal.userStorage.Get"},
		{"package function keeps its package", http.NewRequest, "http.NewRequest"},
		{"interface method names the concrete type", http.DefaultClient.Get, "http.Client.Get"},
		{"exported function of this package", SymbolName, "internal.SymbolName"},
		{"builtin type", 32, "int"},
		{"a string passes through", "user.service.Login", "user.service.Login"},
		{"a path keeps its last element", "github.com/titpetric/oida/user.Login", "user.Login"},
		{"nil is legal but useless", nil, "<nil>"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := SymbolName(testCase.input); got != testCase.want {
				t.Fatalf("SymbolName(%v) = %q, want %q", testCase.input, got, testCase.want)
			}
		})
	}
}
