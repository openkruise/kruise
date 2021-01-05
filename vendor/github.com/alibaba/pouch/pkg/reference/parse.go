package reference

import (
	"errors"

	digest "github.com/opencontainers/go-digest"
)

var (
	// ErrInvalid is used to return error if reference is invalid
	ErrInvalid = errors.New("invalid reference")

	// defaultTag is latest if there is no tag
	defaultTag = "latest"
)

// Parse parses ref into reference.Named.
func Parse(ref string) (Named, error) {
	if ok := regRef.MatchString(ref); !ok {
		return nil, ErrInvalid
	}

	name, tag, digStr := splitReference(ref)
	namedRef := namedReference{name}

	if digStr != "" {
		dig, err := digest.Parse(digStr)
		if err != nil {
			return nil, err
		}

		if tag == "" {
			return canonicalDigestedReference{
				Named:  namedRef,
				digest: dig,
			}, nil
		}

		return reference{
			Named:  namedRef,
			tag:    tag,
			digest: dig,
		}, nil
	}

	if tag != "" {
		return taggedReference{
			Named: namedRef,
			tag:   tag,
		}, nil
	}
	return namedRef, nil
}

// WithDefaultTagIfMissing adds default tag "latest" for the Named reference.
func WithDefaultTagIfMissing(named Named) Named {
	if IsNamedOnly(named) {
		return taggedReference{
			Named: named,
			tag:   defaultTag,
		}
	}
	return named
}

// WithTag adds tag for the Named reference.
func WithTag(named Named, tag string) Named {
	return taggedReference{
		Named: named,
		tag:   tag,
	}
}

// WithDigest adds digest for the Named reference.
func WithDigest(named Named, dig digest.Digest) Named {
	return canonicalDigestedReference{
		Named:  named,
		digest: dig,
	}
}

// TrimTagForDigest removes the tag information if the Named reference is digest.
func TrimTagForDigest(named Named) Named {
	if digRef, ok := named.(Digested); ok {
		return WithDigest(named, digRef.Digest())
	}
	return named
}

// IsNamedOnly return true if the ref is the Named without tag or digest.
func IsNamedOnly(ref Named) bool {
	if _, ok := ref.(Tagged); ok {
		return false
	}

	if _, ok := ref.(CanonicalDigested); ok {
		return false
	}
	return true
}

// IsCanonicalDigested return true if the ref is the canonical digested reference.
func IsCanonicalDigested(ref Named) bool {
	if _, ok := ref.(Tagged); ok {
		return false
	}

	_, ok := ref.(CanonicalDigested)
	return ok
}

// IsNameTagged return true if the ref is the Named with tag.
func IsNameTagged(ref Named) bool {
	if _, ok := ref.(Digested); ok {
		return false
	}

	_, ok := ref.(Tagged)
	return ok
}

// splitReference splits reference into name, tag and digest in string format.
func splitReference(ref string) (name string, tag string, digStr string) {
	name = ref

	if loc := regDigest.FindStringIndex(name); loc != nil {
		name, digStr = name[:loc[0]], name[loc[0]+1:]
	}

	if loc := regTag.FindStringIndex(name); loc != nil {
		name, tag = name[:loc[0]], name[loc[0]+1:]
	}
	return
}
