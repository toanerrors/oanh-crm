import test from 'node:test';
import assert from 'node:assert/strict';
import { dashboardModel, chartBar, chartDomain } from '../src/features/dashboard/dashboard-model.ts';
import { initialWorkspace } from '../src/shared/data/demo.ts';

test('tổng quan dùng số liệu cả kỳ từ server, không cộng riêng 100 chứng từ',()=>{
 const data=structuredClone(initialWorkspace);
 data.documents=[];
 data.server={revenue:'9007199254740993',expense:'70',receivable:'300',payable:'200',moneyBalance:'9223372036854775807',inventoryBalance:'9007199254740993',bankBalance:'10',unbalancedDraftCount:7,balances:{},documentCount:500,postedCount:200,draftCount:300,months:[{key:'2026-10',revenue:'9007199254740993',expense:'70'}]};
 const m=dashboardModel(data,'2026-10');
 assert.equal(m.revenue,9007199254740993n);assert.equal(m.money,9223372036854775807n);assert.equal(m.payable,200n);assert.equal(m.inventory,9007199254740993n);assert.equal(m.unbalanced,7);assert.equal(m.count,500);assert.equal(m.documents.length,0);
 delete data.server.payable;delete data.server.moneyBalance;delete data.server.inventoryBalance;delete data.server.unbalancedDraftCount;
 const missing=dashboardModel(data,'2026-10');assert.equal(missing.payable,null);assert.equal(missing.money,null);assert.equal(missing.inventory,null);assert.equal(missing.unbalanced,null);
});
test('demo giữ chính xác nguồn đã ghi sổ, tháng chọn và dữ liệu mẫu',()=>{
 const m=dashboardModel(initialWorkspace,'2026-10');
 assert.equal(m.revenue,528000000n);assert.equal(m.expense,198000000n);assert.equal(m.money,1279000000n);assert.equal(m.months.length,6);assert.equal(m.months[0].key,'2026-05');assert.equal(m.months[5].key,'2026-10');
 assert.equal(dashboardModel(initialWorkspace,'2026-02').months[0].key,'2025-09');
});
test('biểu đồ giữ số âm, số lớn và vị trí đường 0 thay vì ép về 0',()=>{
 const domain=chartDomain([9007199254740993n,-9007199254740993n,0n]);
 assert.equal(domain.zero,68);assert.deepEqual(chartBar(9007199254740993n,domain),{y:8,height:60});assert.deepEqual(chartBar(-9007199254740993n,domain),{y:68,height:60});
 assert.equal(chartDomain([0n,0n]).empty,true);assert.equal(chartDomain([-10n]).zero,8);assert.equal(chartDomain([10n]).zero,128);
});

test('chứng từ gần đây giữ thứ tự server, tách số dư khỏi hoạt động trong kỳ',()=>{
 const data=structuredClone(initialWorkspace),doc=data.documents[0];
 data.server={revenue:'0',expense:'0',receivable:'0',balances:{},documentCount:2,postedCount:2,draftCount:0,months:[]};
 data.documents=[{...doc,id:'z',date:'2026-10-15'},{...doc,id:'opening-change',date:'2026-10-15',sourceType:'opening_adjustment'},{...doc,id:'a',date:'2026-10-15'}];
 assert.deepEqual(dashboardModel(data,'2026-10').documents.map(d=>d.id),['z','a']);
});
