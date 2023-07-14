/*
 * Copyright 2020 The Yorkie Authors. All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package document provides JSON-like document(CRDT) implementation.
package document

import (
	"fmt"

	"github.com/yorkie-team/yorkie/pkg/document/change"
	"github.com/yorkie-team/yorkie/pkg/document/crdt"
	"github.com/yorkie-team/yorkie/pkg/document/innerpresence"
	"github.com/yorkie-team/yorkie/pkg/document/json"
	"github.com/yorkie-team/yorkie/pkg/document/key"
	"github.com/yorkie-team/yorkie/pkg/document/presence"
	"github.com/yorkie-team/yorkie/pkg/document/time"
)

// DocEvent represents the event that occurred in the document.
type DocEvent struct {
	Type      DocEventType
	Presences map[string]innerpresence.Presence
}

// DocEventType represents the type of the event that occurred in the document.
type DocEventType string

const (
	// WatchedEvent means that the client has established a connection with the server,
	// enabling real-time synchronization.
	WatchedEvent DocEventType = "watched"

	// PresenceChangedEvent means that the presences of the clients who are editing
	// the document have changed.
	PresenceChangedEvent DocEventType = "presence-changed"
)

// Document represents a document accessible to the user.
//
// How document works:
// The operations are generated by the proxy while executing user's command on
// the clone. Then the operations will apply the changes into the base json
// root. This is to protect the base json from errors that may occur while user
// edit the document.
type Document struct {
	// doc is the original data of the actual document.
	doc *InternalDocument

	// cloneRoot is a copy of `doc.root` to be exposed to the user and is used to
	// protect `doc.root`.
	cloneRoot *crdt.Root

	// clonePresences is a copy of `doc.presences` to be exposed to the user and
	// is used to protect `doc.presences`.
	clonePresences *innerpresence.Map

	// events is the channel to send events that occurred in the document.
	events chan DocEvent
}

// New creates a new instance of Document.
func New(key key.Key) *Document {
	return &Document{
		doc:    NewInternalDocument(key),
		events: make(chan DocEvent, 1),
	}
}

// Update executes the given updater to update this document.
func (d *Document) Update(
	updater func(root *json.Object, p *presence.Presence) error,
	msgAndArgs ...interface{},
) error {
	if d.doc.status == StatusRemoved {
		return ErrDocumentRemoved
	}

	if err := d.ensureClone(); err != nil {
		return err
	}

	ctx := change.NewContext(
		d.doc.changeID.Next(),
		messageFromMsgAndArgs(msgAndArgs...),
		d.cloneRoot,
	)

	if err := updater(
		json.NewObject(ctx, d.cloneRoot.Object()),
		presence.New(ctx, d.clonePresences.LoadOrStore(d.ActorID().String(), innerpresence.NewPresence())),
	); err != nil {
		// drop cloneRoot because it is contaminated.
		d.cloneRoot = nil
		d.clonePresences = nil
		return err
	}

	if ctx.HasChange() {
		c := ctx.ToChange()
		if err := c.Execute(d.doc.root, d.doc.presences); err != nil {
			return err
		}

		d.doc.localChanges = append(d.doc.localChanges, c)
		d.doc.changeID = ctx.ID()
	}

	return nil
}

// ApplyChangePack applies the given change pack into this document.
func (d *Document) ApplyChangePack(pack *change.Pack) error {
	// 01. Apply remote changes to both the cloneRoot and the document.
	if len(pack.Snapshot) > 0 {
		d.cloneRoot = nil
		d.clonePresences = nil
		if err := d.doc.applySnapshot(pack.Snapshot, pack.Checkpoint.ServerSeq); err != nil {
			return err
		}
	} else {
		if err := d.ensureClone(); err != nil {
			return err
		}

		for _, c := range pack.Changes {
			if err := c.Execute(d.cloneRoot, d.clonePresences); err != nil {
				return err
			}
		}

		events, err := d.doc.ApplyChanges(pack.Changes...)
		if err != nil {
			return err
		}

		for _, e := range events {
			d.events <- e
		}
	}

	// 02. Remove local changes applied to server.
	for d.HasLocalChanges() {
		c := d.doc.localChanges[0]
		if c.ClientSeq() > pack.Checkpoint.ClientSeq {
			break
		}
		d.doc.localChanges = d.doc.localChanges[1:]
	}

	// 03. Update the checkpoint.
	d.doc.checkpoint = d.doc.checkpoint.Forward(pack.Checkpoint)

	// 04. Do Garbage collection.
	d.GarbageCollect(pack.MinSyncedTicket)

	// 05. Update the status.
	if pack.IsRemoved {
		d.SetStatus(StatusRemoved)
	}

	return nil
}

// InternalDocument returns the internal document.
func (d *Document) InternalDocument() *InternalDocument {
	return d.doc
}

// Key returns the key of this document.
func (d *Document) Key() key.Key {
	return d.doc.key
}

// Checkpoint returns the checkpoint of this document.
func (d *Document) Checkpoint() change.Checkpoint {
	return d.doc.checkpoint
}

// HasLocalChanges returns whether this document has local changes or not.
func (d *Document) HasLocalChanges() bool {
	return d.doc.HasLocalChanges()
}

// Marshal returns the JSON encoding of this document.
func (d *Document) Marshal() string {
	return d.doc.Marshal()
}

// CreateChangePack creates pack of the local changes to send to the server.
func (d *Document) CreateChangePack() *change.Pack {
	return d.doc.CreateChangePack()
}

// SetActor sets actor into this document. This is also applied in the local
// changes the document has.
func (d *Document) SetActor(actor *time.ActorID) {
	d.doc.SetActor(actor)
}

// ActorID returns ID of the actor currently editing the document.
func (d *Document) ActorID() *time.ActorID {
	return d.doc.ActorID()
}

// SetStatus updates the status of this document.
func (d *Document) SetStatus(status StatusType) {
	d.doc.SetStatus(status)
}

// Status returns the status of this document.
func (d *Document) Status() StatusType {
	return d.doc.status
}

// IsAttached returns whether this document is attached or not.
func (d *Document) IsAttached() bool {
	return d.doc.IsAttached()
}

// RootObject returns the internal root object of this document.
func (d *Document) RootObject() *crdt.Object {
	return d.doc.RootObject()
}

// Root returns the root object of this document.
func (d *Document) Root() *json.Object {
	if err := d.ensureClone(); err != nil {
		panic(err)
	}

	ctx := change.NewContext(d.doc.changeID.Next(), "", d.cloneRoot)
	return json.NewObject(ctx, d.cloneRoot.Object())
}

// GarbageCollect purge elements that were removed before the given time.
func (d *Document) GarbageCollect(ticket *time.Ticket) int {
	if d.cloneRoot != nil {
		if _, err := d.cloneRoot.GarbageCollect(ticket); err != nil {
			panic(err)
		}
	}

	n, err := d.doc.GarbageCollect(ticket)
	if err != nil {
		panic(err)
	}

	return n
}

// GarbageLen returns the count of removed elements.
func (d *Document) GarbageLen() int {
	return d.doc.GarbageLen()
}

func (d *Document) ensureClone() error {
	if d.cloneRoot == nil {
		copiedDoc, err := d.doc.root.DeepCopy()
		if err != nil {
			return err
		}
		d.cloneRoot = copiedDoc
	}

	if d.clonePresences == nil {
		d.clonePresences = d.doc.presences.DeepCopy()
	}

	return nil
}

// Presences returns the presence map of this document.
func (d *Document) Presences() map[string]innerpresence.Presence {
	// TODO(hackerwins): We need to use client key instead of actor ID for exposing presence.
	presences := make(map[string]innerpresence.Presence)
	d.doc.presences.Range(func(key string, value innerpresence.Presence) bool {
		presences[key] = value
		return true
	})
	return presences
}

// Presence returns the presence of the given client.
func (d *Document) Presence(clientID string) innerpresence.Presence {
	return d.doc.Presence(clientID)
}

// MyPresence returns the presence of the actor.
func (d *Document) MyPresence() innerpresence.Presence {
	return d.doc.MyPresence()
}

// SetOnlineClientSet sets the online client set.
func (d *Document) SetOnlineClientSet(clientIDs ...string) {
	d.doc.SetOnlineClientSet(clientIDs...)
}

// AddOnlineClient adds the given client to the online client set.
func (d *Document) AddOnlineClient(clientID string) {
	d.doc.AddOnlineClient(clientID)
}

// RemoveOnlineClient removes the given client from the online client set.
func (d *Document) RemoveOnlineClient(clientID string) {
	d.doc.RemoveOnlineClient(clientID)
}

// OnlinePresence returns the presence of the given client. If the client is not
// online, it returns nil.
func (d *Document) OnlinePresence(clientID string) innerpresence.Presence {
	return d.doc.OnlinePresence(clientID)
}

// Events returns the events of this document.
func (d *Document) Events() <-chan DocEvent {
	return d.events
}

func messageFromMsgAndArgs(msgAndArgs ...interface{}) string {
	if len(msgAndArgs) == 0 {
		return ""
	}
	if len(msgAndArgs) == 1 {
		msg := msgAndArgs[0]
		if msgAsStr, ok := msg.(string); ok {
			return msgAsStr
		}
		return fmt.Sprintf("%+v", msg)
	}
	if len(msgAndArgs) > 1 {
		return fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
	}
	return ""
}
