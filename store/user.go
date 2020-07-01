package store

// User holds user-related info
type User struct {
	ID       string `json:"id" storm:"id"`
	Name     string `json:"name"`
	Picture  string `json:"picture"`
	Provider string `json:"provider"`
	// username and password are used only by local provider
	Username     string      `json:"username" storm:"index"`
	Password     string      `json:"password"`
	Scope        string      `json:"scope"`
	Locale       string      `json:"locale"`
	Rules        []Rule      `json:"rules"`
	Commands     []string    `json:"commands"`
	LockPassword bool        `json:"lockPassword"`
	Permissions  Permissions `json:"permissions"`
	Admin        bool        `json:"admin"`
	Blocked      bool        `json:"blocked"`
}

// Permissions describe a user's permissions.
type Permissions struct {
	Execute  bool `json:"execute"`
	Create   bool `json:"create"`
	Rename   bool `json:"rename"`
	Modify   bool `json:"modify"`
	Delete   bool `json:"delete"`
	Share    bool `json:"share"`
	Download bool `json:"download"`
}

// Rule is a allow/disallow rule.
type Rule struct {
	Regex bool   `json:"regex"`
	Allow bool   `json:"allow"`
	Rule  string `json:"rule"`
}
