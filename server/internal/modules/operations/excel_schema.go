package operations

import (
	"strings"
	"unicode"
)

type ExcelField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Basic    bool     `json:"basic"`
	Choices  []string `json:"choices,omitempty"`
	Default  string   `json:"default,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
}
type ExcelSchema struct {
	Key     string       `json:"key"`
	Label   string       `json:"label"`
	Group   string       `json:"group"`
	Owner   bool         `json:"owner"`
	Version int          `json:"version"`
	Fields  []ExcelField `json:"fields"`
}

func excelField(key, label, kind string, required bool, choices ...string) ExcelField {
	return ExcelField{Key: key, Label: label, Type: kind, Required: required, Basic: required, Choices: choices}
}
func excelDefault(key, label, kind, value string) ExcelField {
	f := excelField(key, label, kind, false)
	f.Default = value
	return f
}
func ExcelSchemas(role string) []ExcelSchema {
	f := excelField
	active := excelDefault("active", "Đang sử dụng", "bool", "true")
	code := f("code", "Mã", "code", true)
	name := f("name", "Tên", "text", true)
	schemas := []ExcelSchema{
		{Key: "partners", Label: "Khách hàng, nhà cung cấp", Group: "Danh mục", Fields: []ExcelField{code, name, f("role", "Vai trò", "choice", true, "customer", "supplier", "both"), f("email", "Email", "text", false), f("phone", "Điện thoại", "identifier", false), active}},
		{Key: "accounts", Label: "Tài khoản kế toán", Group: "Danh mục", Owner: true, Fields: []ExcelField{code, name, f("parentCode", "Mã tài khoản cha", "code", false), f("group", "Nhóm tài khoản", "choice", true, "cash", "receivable", "payable", "revenue", "expense", "inventory", "capital", "asset", "tax", "liability", "wip"), f("requiresPartner", "Yêu cầu đối tượng", "bool", true), active}},
		{Key: "items", Label: "Hàng hóa, dịch vụ", Group: "Danh mục", Fields: []ExcelField{code, name, f("unit", "Đơn vị tính", "text", true), f("tracked", "Theo dõi tồn kho", "bool", true), active}},
		{Key: "warehouses", Label: "Kho", Group: "Danh mục", Fields: []ExcelField{code, name, active}},
		{Key: "banks", Label: "Tài khoản ngân hàng", Group: "Danh mục", Fields: []ExcelField{f("number", "Số tài khoản ngân hàng", "identifier", true), f("bankName", "Tên ngân hàng", "text", true), f("accountCode", "Tài khoản kế toán", "code", true), active}},
		{Key: "employees", Label: "Nhân viên", Group: "Danh mục", Owner: true, Fields: []ExcelField{code, name, f("baseSalary", "Lương cơ bản", "money", true), f("insuranceBase", "Lương đóng bảo hiểm", "money", true), f("dependents", "Số người phụ thuộc", "integer", true), active}},
	}
	common := []ExcelField{f("sourceId", "Mã chứng từ nguồn", "code", true), f("date", "Ngày hạch toán", "date", true), f("description", "Diễn giải", "text", true), f("partnerCode", "Mã đối tượng", "code", false)}
	info := []ExcelField{f("documentDate", "Ngày chứng từ", "date", false), f("contact", "Người nộp, người nhận", "text", false), f("address", "Địa chỉ", "text", false), f("taxCode", "Mã số thuế", "identifier", false), f("employee", "Nhân viên phụ trách", "text", false), f("reference", "Số tham chiếu", "text", false), f("contractNumber", "Số hợp đồng", "text", false), f("notes", "Ghi chú", "text", false), f("originalCount", "Số chứng từ gốc", "integer", false), f("invoiceLookupCode", "Mã tra cứu hóa đơn", "text", false), f("invoiceLookupUrl", "Đường dẫn tra cứu hóa đơn", "text", false)}
	add := func(key, label string, fields []ExcelField) {
		all := append([]ExcelField{}, common...)
		if key == "sale" || key == "purchase" {
			all[3].Required = true
			all[3].Basic = true
		}
		all = append(all, fields...)
		if key != "journal-pairs" && key != "journal-lines" {
			all = append(all, info...)
		}
		schemas = append(schemas, ExcelSchema{Key: key, Label: label, Group: "Chứng từ", Fields: all})
	}
	add("journal-pairs", "Chứng từ khác · cặp Nợ/Có", []ExcelField{f("debitAccount", "TK Nợ", "code", true), f("debitPartner", "Đối tượng Nợ", "code", false), f("creditAccount", "TK Có", "code", true), f("creditPartner", "Đối tượng Có", "code", false), f("amount", "Số tiền", "money", true), f("lineDescription", "Diễn giải dòng", "text", false)})
	add("journal-lines", "Chứng từ khác · nhiều dòng Nợ/Có", []ExcelField{f("accountCode", "Tài khoản", "code", true), f("linePartnerCode", "Đối tượng hạch toán", "code", false), excelDefault("debit", "Số tiền Nợ", "money", "0"), excelDefault("credit", "Số tiền Có", "money", "0"), f("lineDescription", "Diễn giải dòng", "text", false)})
	treasury := []ExcelField{f("moneyAccount", "Tài khoản tiền", "code", true), f("bankNumber", "Số tài khoản ngân hàng", "identifier", false), f("counterAccount", "Tài khoản đối ứng", "code", false), f("linePartnerCode", "Đối tượng hạch toán", "code", false), f("amount", "Số tiền", "money", true), f("lineDescription", "Diễn giải dòng", "text", false)}
	treasury = append(treasury, f("invoiceSourceId", "Mã nguồn công nợ hoặc số chứng từ gốc", "code", false), f("allocationAmount", "Số tiền phân bổ công nợ", "money", false))
	add("receipt", "Phiếu thu, thu tiền gửi", treasury)
	add("payment", "Phiếu chi, chi tiền gửi", treasury)
	add("transfer", "Chuyển tiền nội bộ", []ExcelField{f("moneyAccount", "Tài khoản chuyển", "code", true), f("bankNumber", "Số tài khoản chuyển", "identifier", false), f("destinationAccount", "Tài khoản nhận", "code", true), f("destinationBankNumber", "Số tài khoản nhận", "identifier", false), f("amount", "Số tiền", "money", true)})
	trade := []ExcelField{f("tradeMode", "Loại mua bán", "choice", true, "goods", "service"), f("priceMode", "Cách tính giá", "choice", true, "exclusive", "inclusive"), f("itemCode", "Mã hàng hóa, dịch vụ", "code", true), f("warehouseCode", "Mã kho", "code", false), f("quantity", "Số lượng", "decimal", true), f("unitPrice", "Đơn giá", "decimal", true), f("vatRate", "Thuế suất GTGT (%)", "decimal", true), f("lineDescription", "Diễn giải dòng", "text", false), f("accountCode", "Tài khoản dòng", "code", false), f("invoiceNumber", "Số hóa đơn", "identifier", false), f("invoiceSeries", "Ký hiệu hóa đơn", "text", false), f("invoiceDate", "Ngày hóa đơn", "date", false), f("dueDate", "Hạn thanh toán", "date", false), f("paymentAccount", "Tài khoản thanh toán ngay", "code", false), f("bankNumber", "Số tài khoản thanh toán", "identifier", false), f("discountRate", "Tỷ lệ chiết khấu dòng (%)", "decimal", false), f("discountAmount", "Tiền chiết khấu dòng", "money", false), f("discountMode", "Chiết khấu hóa đơn", "choice", false, "none", "line", "percent", "total"), f("invoiceDiscountRate", "Tỷ lệ CK hóa đơn (%)", "decimal", false), f("invoiceDiscountAmount", "Tiền CK hóa đơn", "money", false), f("discountAllocation", "Phân bổ CK hóa đơn", "choice", false, "value", "quantity", "manual")}
	sale := append([]ExcelField{}, trade...)
	add("sale", "Chứng từ bán hàng, dịch vụ", sale)
	purchase := append([]ExcelField{}, trade...)
	purchase = append(purchase, f("deductible", "Đủ điều kiện khấu trừ VAT", "bool", true), f("isPurchaseCost", "Là chi phí mua hàng", "bool", false), f("costAllocation", "Phân bổ chi phí mua hàng", "choice", false, "value", "quantity", "percent", "manual"), f("costSourceId", "Mã nguồn chi phí mua hàng", "code", false), f("costSourceLine", "Dòng nguồn chi phí (từ 1)", "integer", false), f("costAmount", "Chi phí lấy từ nguồn", "money", false), f("purchaseCostAmount", "Chi phí phân bổ vào dòng", "money", false), f("costPercent", "Tỷ lệ phân bổ chi phí (%)", "decimal", false))
	add("purchase", "Chứng từ mua hàng, dịch vụ", purchase)
	schemas = append(schemas, ExcelSchema{Key: "opening", Label: "Số dư đầu kỳ", Group: "Số dư ban đầu", Owner: true, Fields: []ExcelField{f("rowKind", "Loại dòng số dư", "choice", true, "account", "stock", "fixed", "prepaid"), f("accountCode", "Tài khoản", "code", false), f("partnerCode", "Mã đối tượng", "code", false), f("bankNumber", "Số tài khoản ngân hàng", "identifier", false), excelDefault("debit", "Dư Nợ", "money", "0"), excelDefault("credit", "Dư Có", "money", "0"), f("itemCode", "Mã hàng", "code", false), f("warehouseCode", "Mã kho", "code", false), f("quantity", "Số lượng tồn", "decimal", false), f("value", "Giá trị tồn", "money", false), f("code", "Mã tài sản, khoản phân bổ", "code", false), f("name", "Tên tài sản, khoản phân bổ", "text", false), f("cost", "Nguyên giá", "money", false), f("allocated", "Đã khấu hao, phân bổ", "money", false), f("residual", "Giá trị thu hồi", "money", false), f("months", "Số tháng phân bổ", "integer", false), f("startPeriod", "Kỳ bắt đầu phân bổ (YYYY-MM)", "period", false)}})
	result := []ExcelSchema{}
	for _, schema := range schemas {
		schema.Version = 1
		if role != "owner" && role != "accountant" || role != "owner" && schema.Owner {
			continue
		}
		for i := range schema.Fields {
			field := &schema.Fields[i]
			if schema.Key == "accounts" && field.Key == "parentCode" {
				field.Basic = true
			}
			if (schema.Key == "receipt" || schema.Key == "payment") && (field.Key == "counterAccount" || field.Key == "bankNumber") {
				field.Basic = true
			}
			field.Aliases = []string{field.Key, field.Label}
			if field.Key == "parentCode" {
				field.Aliases = append(field.Aliases, "TK cha", "Tài khoản cha")
			}
			switch field.Key {
			case "sourceId":
				field.Aliases = append(field.Aliases, "Số chứng từ", "Mã nguồn", "sourceId")
			case "date":
				field.Aliases = append(field.Aliases, "Ngày kế toán", "date")
			case "partnerCode":
				field.Aliases = append(field.Aliases, "Mã khách hàng", "Mã nhà cung cấp", "Đối tượng")
			case "code":
				field.Aliases = append(field.Aliases, "Mã đối tượng")
				field.Aliases = append(field.Aliases, "Mã khách hàng", "Mã nhà cung cấp", "Mã hàng", "Mã kho", "Mã nhân viên", "Mã tài khoản")
			case "name":
				field.Aliases = append(field.Aliases, "Tên đối tượng")
				field.Aliases = append(field.Aliases, "Tên khách hàng", "Tên nhà cung cấp", "Tên hàng", "Tên kho", "Tên nhân viên", "Tên tài khoản")
			}
		}
		result = append(result, schema)
	}
	return result
}
func excelSchema(key string) (ExcelSchema, bool) {
	for _, schema := range ExcelSchemas("owner") {
		if schema.Key == key {
			return schema, true
		}
	}
	return ExcelSchema{}, false
}
func excelHeader(value string) string {
	groups := []string{"aàáạảãâầấậẩẫăằắặẳẵ", "eèéẹẻẽêềếệểễ", "iìíịỉĩ", "oòóọỏõôồốộổỗơờớợởỡ", "uùúụủũưừứựửữ", "yỳýỵỷỹ", "dđ"}
	value = strings.ToLower(strings.TrimSpace(value))
	for _, g := range groups {
		base := []rune(g)[0]
		for _, r := range []rune(g)[1:] {
			value = strings.ReplaceAll(value, string(r), string(base))
		}
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, value)
}
