// Package cachekey is the one list of cache key namespaces.
//
// Every cache key is "<namespace>:<part>:<part>…", and the cache derives the
// file stems it manages (stats, GC, purge) from this list. A namespace exists
// only through register below, so a provider cannot add keys the cache does
// not know about: that is how five providers once went uncollected.
package cachekey

import "strings"

// Namespace is the first segment of a cache key.
type Namespace string

var all []Namespace

func register(name string) Namespace {
	n := Namespace(name)
	all = append(all, n)
	return n
}

// Namespaces in use. Feed is the legacy whole-feed key, kept so purge still
// removes entries written by older versions.
var (
	Profile    = register("profile")
	Timeline   = register("timeline")
	LongText   = register("longtext")
	Domain     = register("domain")
	Hot        = register("hot")
	Feed       = register("feed")
	Sspai      = register("sspai")
	V2ex       = register("v2ex")
	X          = register("x")
	Youtube    = register("youtube")
	Smzdm      = register("smzdm")
	Reddit     = register("reddit")
	Telegram   = register("telegram")
	LinuxDo    = register("linuxdo")
	Douyin     = register("douyin")
	TikTok     = register("tiktok")
	Bilibili   = register("bilibili")
	Pinterest  = register("pinterest")
	Polymarket = register("polymarket")
	Apple      = register("apple")
	Xiaoyuzhou = register("xiaoyuzhou")
)

// All returns every registered namespace.
func All() []Namespace { return append([]Namespace(nil), all...) }

// Key joins the namespace and parts with ':'.
func (n Namespace) Key(parts ...string) string {
	return string(n) + ":" + strings.Join(parts, ":")
}

// Owns reports whether key belongs to the namespace.
func (n Namespace) Owns(key string) bool {
	return strings.HasPrefix(key, string(n)+":")
}

// StemPrefix is the file-stem prefix of the namespace's entries: the cache
// maps ':' (like every other character outside [A-Za-z0-9._-]) to '_'.
func (n Namespace) StemPrefix() string { return string(n) + "_" }
