package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path"

	log "github.com/sirupsen/logrus"

	"github.com/oxzi/gosh/internal"
)

var (
	storePath  string
	verbose    bool
	modeDelete bool

	id        string
	ipAddress net.IP
)

func init() {
	log.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

	var ipAddressStr string

	flag.StringVar(&storePath, "store", "", "Path to the store, env variable GOSHSTORE can also be used")
	flag.BoolVar(&verbose, "verbose", false, "Verbose logging")
	flag.BoolVar(&modeDelete, "delete", false, "Delete selection")
	flag.StringVar(&id, "id", "", "Query for an ID")
	flag.StringVar(&ipAddressStr, "ip-addr", "", "Query for an IP address")

	flag.Parse()

	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	if ipAddressStr != "" {
		ipAddress = net.ParseIP(ipAddressStr)
	}
}

func getStorePath() string {
	if storePath != "" {
		return storePath
	} else if envPath := os.Getenv("GOSHSTORE"); envPath != "" {
		return envPath
	} else {
		return "."
	}
}

func checkStorePath(storep string) (err error) {
	dirs := []string{
		storep,
		path.Join(storep, internal.DirDatabase),
		path.Join(storep, internal.DirStorage),
	}

	for _, dir := range dirs {
		if _, stat := os.Stat(dir); os.IsNotExist(stat) {
			err = fmt.Errorf("required directory %s does not exist", dir)
			return
		}
	}

	return
}

func prettyPrintItem(item internal.Item) {
	log.Infof("### Item: %s", item.ID)
	log.Infof(" - Filename: %s (%s)", item.Filename, item.ContentType)
	log.Infof(" - Burn After Reading: %t", item.BurnAfterReading)
	log.Infof(" - Created: %v", item.Created)
	log.Infof(" - Expires: %v", item.Expires)

	for ipK, ipV := range item.Owner {
		log.Infof(" - IP, %s: %v", ipK, ipV)
	}

	log.Infof("")
}

func main() {
	storep := getStorePath()
	if err := checkStorePath(storep); err != nil {
		log.WithError(err).WithField("path", storep).Fatal("Failed to load store")
	}

	store, err := internal.NewStore(storep, false, false)
	if err != nil {
		log.WithError(err).WithField("path", storep).Fatal("Failed to start store")
	}

	if items, itemsErr := query(store); itemsErr != nil {
		log.WithError(itemsErr).Warn("Failed to execute query")
	} else {
		if modeDelete {
			for _, item := range items {
				log.WithField("ID", item.ID).Info("Deleting Item")

				if err := store.Delete(item); err != nil {
					log.WithError(err).WithField("ID", item.ID).Warn("Deletion errored")
				}
			}
		} else {
			for _, item := range items {
				prettyPrintItem(item)
			}
		}
	}

	if err := store.Close(); err != nil {
		log.WithError(err).Fatal("Closing errored")
	}
}
