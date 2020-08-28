package main

import (
	"errors"
	"net"

	"github.com/oxzi/gosh"
	"github.com/timshannon/badgerhold"
)

func query(store *gosh.Store) (items []gosh.Item, err error) {
	if id != "" {
		return queryId(id, store)
	}

	if ipAddress != nil {
		return queryIpAddr(ipAddress, store)
	}

	err = errors.New("No query was specified")
	return
}

func queryId(id string, store *gosh.Store) (items []gosh.Item, err error) {
	if item, itemErr := store.Get(id, false); itemErr != nil {
		err = itemErr
	} else {
		items = []gosh.Item{item}
	}
	return
}

func queryIpAddr(ipAddress net.IP, store *gosh.Store) (items []gosh.Item, err error) {
	matchIp := func(ra *badgerhold.RecordAccess) (match bool, err error) {
		item := ra.Record().(*gosh.Item)

		for _, ownerMapIp := range item.Owner {
			if ownerMapIp.Equal(ipAddress) {
				match = true
				return
			}
		}
		return
	}

	err = store.BadgerHold().Find(&items, badgerhold.Where("Owner").MatchFunc(matchIp))
	return
}
