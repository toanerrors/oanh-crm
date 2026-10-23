package ledger

type Line struct {
	AccountID   string `json:"accountId"`
	PartnerID   string `json:"partnerId,omitempty"`
	Description string `json:"description"`
	Debit       string `json:"debit"`
	Credit      string `json:"credit"`
	PartnerName string `json:"partnerName,omitempty"`
	AccountName string `json:"accountName,omitempty"`
}
type Document struct {
	Voucher     bool   `json:"voucher,omitempty"`
	ID          string `json:"id"`
	Reference   string `json:"reference"`
	Type        string `json:"type"`
	Date        string `json:"date"`
	Description string `json:"description"`
	PartnerID   string `json:"partnerId,omitempty"`
	Status      string `json:"status"`
	Version     int    `json:"version"`
	SourceType  string `json:"sourceType,omitempty"`
	SourceID    string `json:"sourceId,omitempty"`
	ReversalOf  string `json:"reversalOf,omitempty"`
	PolicyID    string `json:"policyId,omitempty"`
	Lines       []Line `json:"lines"`
}
type Account struct {
	HasReferences   bool    `json:"hasReferences,omitempty"`
	ParentID        *string `json:"parentId,omitempty"`
	ParentRevision  int64   `json:"parentRevision,omitempty"`
	ID              string  `json:"id"`
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	Group           string  `json:"group"`
	RequiresPartner bool    `json:"requiresPartner"`
	Active          bool    `json:"active"`
}
type Partner struct {
	HasReferences bool   `json:"hasReferences,omitempty"`
	ID            string `json:"id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	Active        bool   `json:"active"`
}
type Period struct {
	Period string `json:"period"`
	Closed bool   `json:"closed"`
}
type Context struct {
	ConversionDate string `json:"conversionDate,omitempty"`
	Period         string `json:"period,omitempty"`
	CompanyID      string `json:"companyId"`
	CompanyName    string `json:"companyName"`
	Epoch          string `json:"epoch"`
	Revision       int64  `json:"revision"`
	Currency       string `json:"currency"`
	Scale          int    `json:"scale"`
	Regime         string `json:"regime"`
}
type Workspace struct {
	Server *Metrics `json:"server"`
	Context
	Actor       string     `json:"actor"`
	Permissions string     `json:"permissions"`
	Mode        string     `json:"mode"`
	Documents   []Document `json:"documents"`
	Accounts    []Account  `json:"accounts"`
	Partners    []Partner  `json:"partners"`
	Periods     []Period   `json:"periods"`
}
type Command struct {
	Actor string
	Epoch string
	Key   string
}
type Post struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}
type Reversal struct {
	ID     string `json:"id"`
	Date   string `json:"date"`
	Reason string `json:"reason"`
}
