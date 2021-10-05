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

package document

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/yorkie-team/yorkie/internal/log"
	"github.com/yorkie-team/yorkie/pkg/document/change"
	"github.com/yorkie-team/yorkie/pkg/document/checkpoint"
	"github.com/yorkie-team/yorkie/pkg/document/json"
	"github.com/yorkie-team/yorkie/pkg/document/key"
	"github.com/yorkie-team/yorkie/pkg/document/proxy"
	"github.com/yorkie-team/yorkie/pkg/document/time"
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

	// clone is a copy of `doc` to be exposed to the user and is used to
	// protect `doc`.
	clone *json.Root
}

// New creates a new instance of Document.
func New(collection, document string) *Document {
	return &Document{
		doc: NewInternalDocument(collection, document),
	}
}

// Update executes the given updater to update this document.
func (d *Document) Update(
	updater func(root *proxy.ObjectProxy) error,
	msgAndArgs ...interface{},
) error {
	d.ensureClone()

	ctx := change.NewContext(
		d.doc.changeID.Next(),
		messageFromMsgAndArgs(msgAndArgs),
		d.clone,
	)

	if err := updater(proxy.NewObjectProxy(ctx, d.clone.Object())); err != nil {
		// drop clone because it is contaminated.
		d.clone = nil
		log.Logger.Error(err)
		return err
	}

	if ctx.HasOperations() {
		c := ctx.ToChange()
		if err := c.Execute(d.doc.root); err != nil {
			return err
		}

		d.doc.localChanges = append(d.doc.localChanges, c)
		d.doc.changeID = ctx.ID()
	}

	return nil
}

// ApplyChangePack applies the given change pack into this document.
func (d *Document) ApplyChangePack(pack *change.Pack) error {
	// 01. Apply remote changes to both the clone and the document.
	if len(pack.Snapshot) > 0 {
		d.clone = nil
		if err := d.doc.applySnapshot(pack.Snapshot, pack.Checkpoint.ServerSeq); err != nil {
			return err
		}
	} else {
		d.ensureClone()

		for _, c := range pack.Changes {
			if err := c.Execute(d.clone); err != nil {
				return err
			}
		}

		if err := d.doc.applyChanges(pack.Changes); err != nil {
			return err
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

	if log.Core.Enabled(zap.DebugLevel) {
		log.Logger.Debugf("after apply %d changes: %s", len(pack.Changes), d.RootObject().Marshal())
	}
	return nil
}

// Key returns the key of this document.
func (d *Document) Key() *key.Key {
	return d.doc.key
}

// Checkpoint returns the checkpoint of this document.
func (d *Document) Checkpoint() *checkpoint.Checkpoint {
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

// Actor sets actor.
func (d *Document) Actor() *time.ActorID {
	return d.doc.Actor()
}

// SetStatus updates the status of this document.
func (d *Document) SetStatus(status statusType) {
	d.doc.SetStatus(status)
}

// IsAttached returns the whether this document is attached or not.
func (d *Document) IsAttached() bool {
	return d.doc.IsAttached()
}

// RootObject returns the root object.
func (d *Document) RootObject() *json.Object {
	return d.doc.RootObject()
}

// Root returns the proxy of the root object.
func (d *Document) Root() *proxy.ObjectProxy {
	d.ensureClone()

	ctx := change.NewContext(d.doc.changeID.Next(), "", d.clone)
	return proxy.NewObjectProxy(ctx, d.clone.Object())
}

// GarbageCollect purge elements that were removed before the given time.
func (d *Document) GarbageCollect(ticket *time.Ticket) int {
	if d.clone != nil {
		d.clone.GarbageCollect(ticket)
	}
	return d.doc.GarbageCollect(ticket)
}

// GarbageLen returns the count of removed elements.
func (d *Document) GarbageLen() int {
	return d.doc.GarbageLen()
}

func (d *Document) ensureClone() {
	if d.clone == nil {
		d.clone = d.doc.root.DeepCopy()
	}
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
