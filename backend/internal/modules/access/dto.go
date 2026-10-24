package access

type GroupRef struct {
	ID        string  `json:"id"`
	SystemKey *string `json:"system_key"`
	Name      string  `json:"name"`
}

type Group struct {
	ID          string   `json:"id"`
	SystemKey   *string  `json:"system_key"`
	Name        string   `json:"name"`
	IsPreset    bool     `json:"is_preset"`
	MemberCount int      `json:"member_count"`
	Permissions []string `json:"permissions"`
}

type UserSummary struct {
	ID                      string   `json:"id"`
	Email                   string   `json:"email"`
	Nickname                string   `json:"nickname"`
	EmailVerificationStatus string   `json:"email_verification_status"`
	Group                   GroupRef `json:"group"`
	ExtraPermissions        []string `json:"extra_permissions"`
}

type UserDetail struct {
	UserSummary
	Permissions []string `json:"permissions"`
}

type UpdateGroupInput struct {
	Name        *string
	Permissions *[]string
}

type UpdateUserInput struct {
	GroupID          *string
	ExtraPermissions *[]string
}

type permissionResponse struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type groupRequest struct {
	Name        *string   `json:"name"`
	Permissions *[]string `json:"permissions"`
}

type userRequest struct {
	GroupID          *string   `json:"group_id"`
	ExtraPermissions *[]string `json:"extra_permissions"`
}

type userBatchRequest struct {
	IDs []string `json:"ids"`
}
