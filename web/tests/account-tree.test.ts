import test from 'node:test';
import assert from 'node:assert/strict';
import { accountTree, accountBranchKeys, accountDescendants } from '../src/features/catalog/account-tree.ts';
import type { Account } from '../src/shared/data/types.ts';
const accounts: Account[] = [
 {id:'root',code:'133',name:'Thuế GTGT',group:'tax'},
 {id:'child',code:'1331',name:'Hàng hóa dịch vụ',group:'tax',parentId:'root'},
 {id:'deep',code:'OTHER',name:'Chi tiết thuế',group:'tax',parentId:'child'},
 {id:'independent',code:'1332',name:'Độc lập',group:'tax'},
];
test('cây theo ID đã lưu, không đoán từ tiền tố mã và không thay đầu vào',()=>{
 const tree=accountTree(accounts);assert.equal(tree.length,2);assert.equal(tree[0].children?.[0].children?.[0].code,'OTHER');assert.equal(tree[0].children?.[0].children?.[0].level,3);assert.equal(tree[1].code,'1332');assert.equal(accounts[0].parentId,undefined);assert.deepEqual(accountBranchKeys(tree),['root','child']);
});
test('tìm không dấu giữ chuỗi cha và số con thực tế, loại nhánh không khớp',()=>{
 const tree=accountTree(accounts,'chi tiet thue');assert.equal(tree.length,1);assert.equal(tree[0].code,'133');assert.equal(tree[0].childCount,1);assert.equal(tree[0].children?.[0].children?.[0].id,'deep');assert.match(tree[0].children![0].children![0].path,/133 · Thuế GTGT/);
 assert.equal(accountTree(accounts,'không có').length,0);
});
test('không cho chọn chính tài khoản hoặc nhánh con làm cha; dữ liệu vòng lặp không treo UI',()=>{
 assert.deepEqual([...accountDescendants(accounts,'root')],['root','child','deep']);
 const cycle=accountTree([{...accounts[0],parentId:'child'},accounts[1]]);assert.equal(cycle.length,2);
});
