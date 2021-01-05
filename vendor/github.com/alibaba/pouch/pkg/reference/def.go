package reference

import (
	digest "github.com/opencontainers/go-digest"
)

// Reference represents image name which may include hub/namespace/name:tag
// like registry.hub.docker.com/library/ubuntu:latest.
type Reference interface {
	String() string
}

// Named is an object which has full name.
type Named interface {
	Reference
	Name() string
}

// Tagged is an Named object contains tag.
type Tagged interface {
	Named
	Tag() string
}

// CanonicalDigested is an object which doesn't contains the tag information.
type CanonicalDigested interface {
	Named
	Digest() digest.Digest
}

// Digested is an object which is digest.
type Digested interface {
	Reference
	Digest() digest.Digest
}

// namedReference represents the image short ID or Name.
type namedReference struct {
	name string
}

func (n namedReference) Name() string {
	return n.name
}

func (n namedReference) String() string {
	return n.name
}

// taggedReference represents the image Name:tag.
type taggedReference struct {
	Named
	tag string
}

func (t taggedReference) Tag() string {
	return t.tag
}

func (t taggedReference) String() string {
	return t.Name() + ":" + t.tag
}

// canonicalDigestedReference represents the image canonical digest information.
type canonicalDigestedReference struct {
	Named
	digest digest.Digest
}

func (cd canonicalDigestedReference) String() string {
	return cd.Name() + "@" + cd.digest.String()
}

func (cd canonicalDigestedReference) Digest() digest.Digest {
	return cd.digest
}

type reference struct {
	Named
	tag    string
	digest digest.Digest
}

func (r reference) Tag() string {
	return r.tag
}

func (r reference) Digest() digest.Digest {
	return r.digest
}

func (r reference) String() string {
	return r.Name() + ":" + r.tag + "@" + r.digest.String()
}
