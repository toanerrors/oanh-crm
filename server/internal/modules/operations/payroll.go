package operations

import (
	"context"
	"database/sql"
	"math/big"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
)

func (s *Service) payroll(ctx context.Context, tx *sql.Tx, in Input, p Policy, doc *ledger.Document, result *Result) error {
	var e Employee
	var base, insuranceBase int64
	err := tx.QueryRowContext(ctx, "SELECT id,name,base_salary,insurance_base,dependents,active FROM employees WHERE id=?", in.EmployeeID).Scan(&e.ID, &e.Name, &base, &insuranceBase, &e.Dependents, &e.Active)
	if err != nil || !e.Active {
		return accounting.Fail("INVALID_EMPLOYEE", "Nhân viên không tồn tại hoặc đã ngừng sử dụng.")
	}
	standard, _ := accounting.Decimal(p.StandardDays)
	days, err := accounting.Decimal(in.WorkedDays)
	if err != nil {
		return err
	}
	if days.Cmp(standard) > 0 {
		return accounting.Fail("VALIDATION", "Ngày công vượt ngày chuẩn; nhập giờ làm thêm riêng.")
	}
	wage, err := accounting.Round(new(big.Rat).Quo(new(big.Rat).Mul(accounting.Rat(base), days), standard), p.Rounding)
	if err != nil {
		return err
	}
	allowance, err := accounting.Money(in.Allowance)
	if err != nil {
		return err
	}
	exempt, err := accounting.Money(in.NonTaxableAllowance)
	if err != nil {
		return err
	}
	if exempt > allowance {
		return accounting.Fail("VALIDATION", "Phụ cấp không tính thuế không được vượt tổng phụ cấp.")
	}
	hours, err := accounting.Decimal(in.OvertimeHours)
	if err != nil {
		return err
	}
	multiplier, err := accounting.Decimal(in.OvertimeMultiplier)
	if err != nil {
		return err
	}
	if hours.Sign() > 0 && (multiplier.Sign() <= 0 || multiplier.Cmp(accounting.Rat(10)) > 0) {
		return accounting.Fail("VALIDATION", "Hệ số làm thêm cần lớn hơn 0, không quá 10.")
	}
	hoursPerDay, _ := accounting.Decimal(p.HoursPerDay)
	overtime, err := accounting.Round(new(big.Rat).Quo(new(big.Rat).Mul(new(big.Rat).Mul(accounting.Rat(base), hours), multiplier), new(big.Rat).Mul(standard, hoursPerDay)), p.Rounding)
	if err != nil {
		return err
	}
	gross, err := accounting.Add(wage, allowance, overtime)
	if err != nil {
		return err
	}
	var employeeInsurance, employerInsurance int64
	for _, rate := range p.Insurance {
		employeePart, e := accounting.Percent(accounting.Rat(insuranceBase), rate.EmployeeRate, p.Rounding)
		if e != nil {
			return e
		}
		employerPart, e := accounting.Percent(accounting.Rat(insuranceBase), rate.EmployerRate, p.Rounding)
		if e != nil {
			return e
		}
		employeeInsurance, e = accounting.Add(employeeInsurance, employeePart)
		if e != nil {
			return e
		}
		employerInsurance, e = accounting.Add(employerInsurance, employerPart)
		if e != nil {
			return e
		}
		result.Amounts["insurance.employee."+rate.Name] = accounting.Text(employeePart)
		result.Amounts["insurance.employer."+rate.Name] = accounting.Text(employerPart)
	}
	personal, _ := accounting.Money(p.PersonalDeduction)
	dependent, _ := accounting.Money(p.DependentDeduction)
	taxable := new(big.Int).SetInt64(gross)
	taxable.Sub(taxable, big.NewInt(exempt))
	taxable.Sub(taxable, big.NewInt(employeeInsurance))
	taxable.Sub(taxable, big.NewInt(personal))
	taxable.Sub(taxable, new(big.Int).Mul(big.NewInt(dependent), big.NewInt(int64(e.Dependents))))
	if taxable.Sign() < 0 {
		taxable.SetInt64(0)
	}
	taxRaw := new(big.Rat)
	previous := int64(0)
	remaining := taxable.Int64()
	for _, bracket := range p.PITBrackets {
		part := remaining
		if bracket.UpTo != "" {
			upper, _ := accounting.Money(bracket.UpTo)
			part = min(part, upper-previous)
			previous = upper
		}
		if part > 0 {
			rate, _ := accounting.Decimal(bracket.Rate)
			taxRaw.Add(taxRaw, new(big.Rat).Quo(new(big.Rat).Mul(accounting.Rat(part), rate), accounting.Rat(100)))
			remaining -= part
		}
		if remaining <= 0 {
			break
		}
	}
	tax, err := accounting.Round(taxRaw, p.Rounding)
	if err != nil {
		return err
	}
	deductions, err := accounting.Add(employeeInsurance, tax)
	if err != nil {
		return err
	}
	if deductions > gross {
		return accounting.Fail("INVALID_PAYROLL", "Các khoản khấu trừ vượt tổng thu nhập.")
	}
	expense, err := accounting.Add(gross, employerInsurance)
	if err != nil {
		return err
	}
	insurance, err := accounting.Add(employeeInsurance, employerInsurance)
	if err != nil {
		return err
	}
	addLine(doc, "642", "", "Chi phí lương: "+e.Name, expense, 0)
	addLine(doc, "334", "", "Lương thực lĩnh: "+e.Name, 0, gross-deductions)
	addLine(doc, "338", "", "Bảo hiểm: "+e.Name, 0, insurance)
	addLine(doc, "3335", "", "Thuế TNCN: "+e.Name, 0, tax)
	for k, v := range map[string]int64{"baseSalary": base, "insuranceBase": insuranceBase, "dependentCount": int64(e.Dependents), "gross": gross, "net": gross - deductions, "tax": tax, "taxable": taxable.Int64(), "employeeInsurance": employeeInsurance, "employerInsurance": employerInsurance, "overtime": overtime} {
		result.Amounts[k] = accounting.Text(v)
	}
	result.Notes = append(result.Notes, "Tính theo chính sách doanh nghiệp đã xác nhận; biểu thuế và khoản miễn giảm do người quản trị cấu hình.")
	return nil
}
