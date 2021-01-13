package internal

import (
	"bytes"
	"io"
	"io/ioutil"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"
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

func TestItem(t *testing.T) {
	const maxFilesize = 1024

	tests := []struct {
		size             int64
		filename         string
		burnAfterReading bool
		lifetime         string

		valid bool
	}{
		{0, "", false, "", false},
		{1, "test.jpg", false, "", true},
		{1, "test.jpg", true, "", true},
		{1, "test.jpg", false, "1m", true},
		{1024, "test.jpg", false, "", true},
		{1024, "test.jpg", true, "", true},
		{1024, "test.jpg", true, "23s", true},
		{1024, "", false, "", false},
		{1025, "", false, "", false},
	}

	for _, test := range tests {
		buff := &bytes.Buffer{}
		writer := multipart.NewWriter(buff)

		tmpFileData := make([]byte, test.size)
		rand.Seed(0)
		rand.Read(tmpFileData)

		if f, err := writer.CreateFormFile(formFile, test.filename); err != nil {
			t.Fatal(err)
		} else {
			tmpFileBuff := bytes.NewBuffer(tmpFileData)
			if _, err := io.Copy(f, tmpFileBuff); err != nil {
				t.Fatal(err)
			}
		}

		if test.burnAfterReading {
			if w, err := writer.CreateFormField(formBurnAfterReading); err != nil {
				t.Fatal(err)
			} else if _, err := w.Write([]byte("1")); err != nil {
				t.Fatal(err)
			}
		}

		if test.lifetime != "" {
			if w, err := writer.CreateFormField(formLifetime); err != nil {
				t.Fatal(err)
			} else if _, err := w.Write([]byte(test.lifetime)); err != nil {
				t.Fatal(err)
			}
		}

		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}

		if r, err := http.NewRequest("POST", "http://foo.bar/", buff); err != nil {
			t.Fatal(err)
		} else {
			r.Header.Set("Content-Type", writer.FormDataContentType())
			r.RemoteAddr = "[fe80::42]:2342"

			i, f, err := NewItem(r, maxFilesize, time.Hour, 0)
			if (err == nil) != test.valid {
				t.Fatalf("Is valid: %t, error: %v", test.valid, err)
			}

			i.ID = "test-id-23"

			if !test.valid {
				continue
			}

			if i.Filename != test.filename {
				t.Fatalf("Item Filename mismatches, got %v and expected %v", i.Filename, test.filename)
			}

			if i.BurnAfterReading != test.burnAfterReading {
				t.Fatalf("Burn After Reading mismatches, got %t and expected %t",
					i.BurnAfterReading, test.burnAfterReading)
			}

			if test.lifetime != "" {
				dur, _ := time.ParseDuration(test.lifetime)
				if iDur := i.Expires.Sub(i.Created); iDur != dur {
					t.Fatalf("Expected duration of %v, got %v", dur, iDur)
				}
			} else {
				if iDur := i.Expires.Sub(i.Created); iDur != time.Hour {
					t.Fatalf("Expected duration of %v, got %v", time.Hour, iDur)
				}
			}

			if itemDir, err := ioutil.TempDir("", ""); err != nil {
				t.Fatal(err)
			} else {
				if err := i.WriteFile(f, itemDir); err != nil {
					t.Fatal(err)
				}

				if file, err := i.ReadFile(itemDir); err != nil {
					t.Fatal(err)
				} else if data, err := ioutil.ReadAll(file); err != nil {
					t.Fatal(err)
				} else if !reflect.DeepEqual(tmpFileData, data) {
					t.Fatalf("Data mismatches; got something of length %d and expected %d",
						len(data), len(tmpFileData))
				}

				os.RemoveAll(itemDir)
			}

			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}
