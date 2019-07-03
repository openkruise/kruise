// Package pinterface provides interface types for the xyproto/simple* and xyproto/permission* packages
package pinterface

import "net/http"

// Stable API within the same version number
const Version = 5.0

// Database interfaces

type IList interface {
	Add(value string) error
	All() ([]string, error)        // GetAll in version 4.0
	Last() (string, error)         // GetLast in version 4.0
	LastN(n int) ([]string, error) // GetLastN in version 4.0
	Remove() error
	Clear() error
}

type ISet interface {
	Add(value string) error
	Has(value string) (bool, error)
	All() ([]string, error) // GetAll in version 4.0
	Del(value string) error
	Remove() error
	Clear() error
}

type IHashMap interface {
	Set(owner, key, value string) error
	Get(owner, key string) (string, error)
	Has(owner, key string) (bool, error)
	Exists(owner string) (bool, error)
	All() ([]string, error) // GetAll in version 4.0
	Keys(owner string) ([]string, error)
	DelKey(owner, key string) error
	Del(key string) error
	Remove() error
	Clear() error
}

type IKeyValue interface {
	Set(key, value string) error
	Get(key string) (string, error)
	Del(key string) error
	Inc(key string) (string, error)
	Remove() error
	Clear() error
}

// Interface for making it possible to depend on different versions of the permission package,
// or other packages that implement userstates.
type IUserState interface {
	UserRights(req *http.Request) bool
	HasUser(username string) bool
	BooleanField(username, fieldname string) bool
	SetBooleanField(username, fieldname string, val bool)
	IsConfirmed(username string) bool
	IsLoggedIn(username string) bool
	AdminRights(req *http.Request) bool
	IsAdmin(username string) bool
	UsernameCookie(req *http.Request) (string, error)
	SetUsernameCookie(w http.ResponseWriter, username string) error
	AllUsernames() ([]string, error)
	Email(username string) (string, error)
	PasswordHash(username string) (string, error)
	AllUnconfirmedUsernames() ([]string, error)
	ConfirmationCode(username string) (string, error)
	AddUnconfirmed(username, confirmationCode string)
	RemoveUnconfirmed(username string)
	MarkConfirmed(username string)
	RemoveUser(username string)
	SetAdminStatus(username string)
	RemoveAdminStatus(username string)
	AddUser(username, password, email string)
	SetLoggedIn(username string)
	SetLoggedOut(username string)
	Login(w http.ResponseWriter, username string) error
	ClearCookie(w http.ResponseWriter)
	Logout(username string)
	Username(req *http.Request) string
	CookieTimeout(username string) int64
	SetCookieTimeout(cookieTime int64)
	CookieSecret() string
	SetCookieSecret(cookieSecret string)
	PasswordAlgo() string
	SetPasswordAlgo(algorithm string) error
	HashPassword(username, password string) string
	SetPassword(username, password string)
	CorrectPassword(username, password string) bool
	AlreadyHasConfirmationCode(confirmationCode string) bool
	FindUserByConfirmationCode(confirmationcode string) (string, error)
	Confirm(username string)
	ConfirmUserByConfirmationCode(confirmationcode string) error
	SetMinimumConfirmationCodeLength(length int)
	GenerateUniqueConfirmationCode() (string, error)

	Users() IHashMap
	Host() IHost
	Creator() ICreator
}

// Data structure creator
type ICreator interface {
	NewList(id string) (IList, error)
	NewSet(id string) (ISet, error)
	NewHashMap(id string) (IHashMap, error)
	NewKeyValue(id string) (IKeyValue, error)
}

// Database host (or file)
type IHost interface {
	Ping() error
	Close()
}

// Redis host (implemented structures can also be an IHost, of course)
type IRedisHost interface {
	Pool()
	DatabaseIndex()
}

// Redis data structure creator
type IRedisCreator interface {
	SelectDatabase(dbindex int)
}

// Middleware for permissions
type IPermissions interface {
	SetDenyFunction(f http.HandlerFunc)
	DenyFunction() http.HandlerFunc
	UserState() IUserState
	Clear()
	AddAdminPath(prefix string)
	AddUserPath(prefix string)
	AddPublicPath(prefix string)
	SetAdminPath(pathPrefixes []string)
	SetUserPath(pathPrefixes []string)
	SetPublicPath(pathPrefixes []string)
	Rejected(w http.ResponseWriter, req *http.Request) bool
	ServeHTTP(w http.ResponseWriter, req *http.Request, next http.HandlerFunc)
}
