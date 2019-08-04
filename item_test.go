package main

import (
	"net"
	"net/http"
	"reflect"
	"testing"
)

func TestOwnerType(t *testing.T) {
	header1 := make(http.Header)

	owners1 := make(map[OwnerType]net.IP)
	owners1[RemoteAddr] = net.ParseIP("127.0.0.1")

	header2 := make(http.Header)
	header2.Add(string(Forwarded), "172.23.23.23")
	header2.Add(string(XForwardedFor), "fe80::23")

	owners2 := make(map[OwnerType]net.IP)
	owners2[RemoteAddr] = net.ParseIP("fe80::42")
	owners2[Forwarded] = net.ParseIP("172.23.23.23")
	owners2[XForwardedFor] = net.ParseIP("fe80::23")

	header3 := make(http.Header)
	header3.Add(string(Forwarded), "172.23.23.abc")

	header4 := make(http.Header)
	header4.Add(string(XForwardedFor), "fe80::23")

	tests := []struct {
		remoteAddr string
		headers    http.Header

		ots    map[OwnerType]net.IP
		errors bool
	}{
		{"127.0.0.1:2342", header1, owners1, false},
		{"[fe80::42]:2323", header2, owners2, false},
		{"127.0.0.1:1234", header3, nil, true},
		{"lolwaaat", header4, nil, true},
	}

	for _, test := range tests {
		r := http.Request{
			RemoteAddr: test.remoteAddr,
			Header:     test.headers,
		}

		ots, err := NewOwnerTypes(&r)
		if (err == nil) == test.errors {
			t.Fatalf("Should error: %t, error: %v", test.errors, err)
		}

		if test.errors {
			continue
		}

		if !reflect.DeepEqual(ots, test.ots) {
			t.Fatalf("OwnerTypes are not equal, got %v and expected %v", ots, test.ots)
		}
	}
}
