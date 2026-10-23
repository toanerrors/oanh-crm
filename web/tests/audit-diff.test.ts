import assert from 'node:assert/strict';
import test from 'node:test';
import { auditFields, auditRecordName } from '../src/features/workspace/audit-diff.ts';

test('nhật ký hiển thị tiền lớn chính xác và chỉ các trường thay đổi',()=>{
 const rows=auditFields({table:'document_lines',key:'["doc","0"]',before:{debit:'9007199254740993',credit:'0',description:'Thu'},after:{debit:'9223372036854775807',credit:'0',description:'Thu'}});
 assert.equal(rows.length,1);assert.equal(rows[0].label,'Số tiền Nợ');assert.equal(rows[0].before,'9.007.199.254.740.993');assert.equal(rows[0].after,'9.223.372.036.854.775.807');
});
test('nhật ký phân biệt chưa có, null, trống và số không',()=>{
 const rows=auditFields({table:'partners',key:'["partner"]',before:null,after:{email:'',phone:null,active:'0'}});
 assert.equal(rows.find(r=>r.label==='Email')?.before,'Chưa có');assert.equal(rows.find(r=>r.label==='Email')?.after,'(Trống)');assert.equal(rows.find(r=>r.label==='Điện thoại')?.after,'Không thiết lập');assert.equal(rows.find(r=>r.label==='Sử dụng')?.after,'Ngừng sử dụng');
});
test('biểu mẫu JSON giữ chuỗi tiền và diễn giải được thay đổi lồng nhau',()=>{
 const rows=auditFields({table:'voucher_inputs',key:'["doc"]',before:{payload:JSON.stringify({amount:'9007199254740993',voucher:{notes:'Cũ'}})},after:{payload:JSON.stringify({amount:'9007199254740994',voucher:{notes:'Mới'}})}});
 assert.equal(rows.length,2);assert.equal(rows[0].after,'9.007.199.254.740.994');assert.match(rows[1].label,/Ghi chú/);assert.equal(rows[1].after,'Mới');
});
test('trường kỹ thuật có thể ẩn, danh mục và dòng có nhãn dễ đọc',()=>{
 const rows=auditFields({table:'accounts',key:'["acct"]',before:null,after:{id:'acct',code:'111',name:'Tiền mặt'}});
 assert.equal(rows.find(r=>r.label==='Mã nội bộ')?.technical,true);assert.equal(rows.find(r=>r.label==='Tên')?.technical,false);
 assert.equal(auditRecordName({table:'document_lines',key:'["doc","0"]',before:null,after:{position:'0',debit:'0'}}),'Dòng hạch toán · Dòng 1');
});
