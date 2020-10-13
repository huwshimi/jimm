// Copyright 2020 Canonical Ltd.

package jimmdb

import "gopkg.in/mgo.v2/bson"

// An Update object is used to perform updates to documents in the
// database.
type Update struct {
	AddToSet_ bson.D `bson:"$addToSet,omitempty"`
	Pull_     bson.D `bson:"$pull,omitempty"`
	Set_      bson.M `bson:"$set,omitempty"`
}

// IsZero returns true if this update object is empty, and would therefore
// not make any changes.
func (u *Update) IsZero() bool {
	return len(u.AddToSet_) == 0 && len(u.Pull_) == 0 && len(u.Set_) == 0
}

// AddToSet adds a new $addToSet operation to the update. This will push
// the given value into the given field unless it is already present.
func (u *Update) AddToSet(field string, value interface{}) *Update {
	u.AddToSet_ = append(u.AddToSet_, bson.DocElem{Name: field, Value: value})
	return u
}

// Pull adds a new $pull operation to the update. This will pull all
// instances of the given value from the given field.
func (u *Update) Pull(field string, value interface{}) *Update {
	u.Pull_ = append(u.Pull_, bson.DocElem{Name: field, Value: value})
	return u
}

// Set adds a new $set operation to the update. This will set the given
// field to the given value.
func (u *Update) Set(field string, value interface{}) *Update {
	if u.Set_ == nil {
		u.Set_ = make(bson.M)
	}
	u.Set_[field] = value
	return u
}