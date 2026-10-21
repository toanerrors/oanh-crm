package operations

import "oanh.local/accounting/internal/modules/ledger"

type Insurance struct {
	Name         string `json:"name"`
	EmployeeRate string `json:"employeeRate"`
	EmployerRate string `json:"employerRate"`
}
type Bracket struct {
	UpTo string `json:"upTo"`
	Rate string `json:"rate"`
}
type Policy struct {
	Regime             string      `json:"regime"`
	CorporateTaxRate   string      `json:"corporateTaxRate"`
	ID                 string      `json:"id"`
	Version            int         `json:"version"`
	EffectiveFrom      string      `json:"effectiveFrom"`
	Confirmed          bool        `json:"confirmed"`
	Rounding           string      `json:"rounding"`
	VATMethod          string      `json:"vatMethod"`
	VATRates           []string    `json:"vatRates"`
	InventoryMethod    string      `json:"inventoryMethod"`
	QuantityScale      int         `json:"quantityScale"`
	StandardDays       string      `json:"standardDays"`
	HoursPerDay        string      `json:"hoursPerDay"`
	PersonalDeduction  string      `json:"personalDeduction"`
	DependentDeduction string      `json:"dependentDeduction"`
	Insurance          []Insurance `json:"insurance"`
	PITBrackets        []Bracket   `json:"pitBrackets"`
	DepreciationMethod string      `json:"depreciationMethod"`
	ResidualMethod     string      `json:"residualMethod"`
}
type Item struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Unit     string `json:"unit"`
	Tracked  bool   `json:"tracked"`
	Active   bool   `json:"active"`
	Quantity string `json:"quantity"`
	Value    string `json:"value"`
	Scale    int    `json:"scale"`
}
type Employee struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	BaseSalary    string `json:"baseSalary"`
	InsuranceBase string `json:"insuranceBase"`
	Dependents    int    `json:"dependents"`
	Active        bool   `json:"active"`
}
type Asset struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Cost        string `json:"cost"`
	Residual    string `json:"residual"`
	Months      int    `json:"months"`
	StartPeriod string `json:"startPeriod"`
	Allocated   string `json:"allocated"`
}
type TradeLine struct {
	Deductible *bool  `json:"deductible,omitempty"`
	ItemID     string `json:"itemId"`
	Quantity   string `json:"quantity"`
	UnitPrice  string `json:"unitPrice"`
	VATRate    string `json:"vatRate"`
}
type Allocation struct {
	DocumentID string `json:"documentId"`
	Amount     string `json:"amount"`
}
type Input struct {
	OriginalDocumentID  string       `json:"originalDocumentId,omitempty"`
	MoneyAccount        string       `json:"moneyAccount,omitempty"`
	CounterAccount      string       `json:"counterAccount,omitempty"`
	DestinationAccount  string       `json:"destinationAccount,omitempty"`
	Allocations         []Allocation `json:"allocations,omitempty"`
	InvoiceNumber       string       `json:"invoiceNumber,omitempty"`
	InvoiceSeries       string       `json:"invoiceSeries,omitempty"`
	InvoiceDate         string       `json:"invoiceDate,omitempty"`
	DueDate             string       `json:"dueDate,omitempty"`
	TaxFrom             string       `json:"taxFrom,omitempty"`
	NonDeductible       string       `json:"nonDeductible,omitempty"`
	ExemptIncome        string       `json:"exemptIncome,omitempty"`
	CarriedLoss         string       `json:"carriedLoss,omitempty"`
	ExpectedPolicyID    string       `json:"expectedPolicyId,omitempty"`
	ExpectedRevision    *int64       `json:"expectedRevision,omitempty"`
	Kind                string       `json:"kind"`
	SourceID            string       `json:"sourceId"`
	Date                string       `json:"date"`
	Description         string       `json:"description"`
	PartnerID           string       `json:"partnerId"`
	PriceMode           string       `json:"priceMode"`
	Lines               []TradeLine  `json:"lines"`
	EmployeeID          string       `json:"employeeId"`
	WorkedDays          string       `json:"workedDays"`
	Allowance           string       `json:"allowance"`
	NonTaxableAllowance string       `json:"nonTaxableAllowance"`
	OvertimeHours       string       `json:"overtimeHours"`
	OvertimeMultiplier  string       `json:"overtimeMultiplier"`
	Asset               Asset        `json:"asset"`
	AssetID             string       `json:"assetId"`
	Amount              string       `json:"amount"`
}
type Result struct {
	Revision      int64             `json:"revision"`
	PolicyID      string            `json:"policyId"`
	PolicyVersion int               `json:"policyVersion"`
	Document      ledger.Document   `json:"document"`
	Amounts       map[string]string `json:"amounts"`
	Notes         []string          `json:"notes"`
}
type Data struct {
	Policies  []Policy   `json:"policies"`
	Items     []Item     `json:"items"`
	Employees []Employee `json:"employees"`
	Assets    []Asset    `json:"assets"`
}
