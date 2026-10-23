import test from 'node:test';
import assert from 'node:assert/strict';
import { aimsAtSidebarPanel, SidebarHover } from '../src/app/sidebar-hover.ts';

const panel = { left: 230, top: 200, bottom: 650 };
test('rê chéo sang bảng chức năng giữ ý định, rê dọc chuyển mục nhanh', () => {
  assert.equal(aimsAtSidebarPanel({x:100,y:400},{x:180,y:450},panel),true);
  assert.equal(aimsAtSidebarPanel({x:100,y:400},{x:100,y:450},panel),false);
  assert.equal(aimsAtSidebarPanel({x:100,y:400},{x:90,y:420},panel),false);
  assert.equal(aimsAtSidebarPanel({x:100,y:400},{x:140,y:650},panel),false);
  assert.equal(aimsAtSidebarPanel(undefined,{x:180,y:450},panel),false);
});
test('lướt qua mục nhanh không mở hoặc tải phân hệ ngoài ý định', t => {
  t.mock.timers.enable({apis:['setTimeout']});
  const changes:string[]=[];const hover=new SidebarHover(path=>changes.push(path));
  hover.enter('/cash',{x:100,y:200});t.mock.timers.tick(40);
  hover.leave('/cash');hover.enter('/bank',{x:100,y:240});t.mock.timers.tick(80);
  assert.deepEqual(changes,['/bank']);
  t.mock.timers.tick(500);assert.deepEqual(changes,['/bank']);
});
test('rời mục cũ không được đóng menu mới, trở lại bảng hủy đóng đang chờ', t => {
  t.mock.timers.enable({apis:['setTimeout']});
  const changes:string[]=[];const hover=new SidebarHover(path=>changes.push(path));
  hover.show('/cash');hover.leave('/cash');hover.enter('/bank',{x:100,y:240});t.mock.timers.tick(100);
  hover.leave('/cash');t.mock.timers.tick(500);assert.equal(hover.active,'/bank');
  hover.leave('/bank');t.mock.timers.tick(100);hover.keep('/bank');t.mock.timers.tick(500);
  assert.deepEqual(changes,['/cash','/bank']);
  hover.leave('/bank');t.mock.timers.tick(240);assert.equal(hover.active,'');
});
test('đi chéo qua mục bên cạnh rồi vào bảng cũ không đổi menu', t => {
  t.mock.timers.enable({apis:['setTimeout']});
  const changes:string[]=[];const hover=new SidebarHover(path=>changes.push(path));
  hover.show('/inventory');hover.track('/inventory',{x:100,y:400});hover.leave('/inventory',{x:180,y:450});
  hover.enter('/receivables',{x:180,y:450},panel);t.mock.timers.tick(200);
  assert.equal(hover.active,'/inventory');hover.leave('/receivables');hover.keep('/inventory');t.mock.timers.tick(500);
  assert.deepEqual(changes,['/inventory']);
});
test('điều hướng, thu gọn và Escape hủy mọi lần mở trễ', t => {
  t.mock.timers.enable({apis:['setTimeout']});
  const hover=new SidebarHover(()=>{});
  hover.enter('/sales',{x:100,y:300});hover.show('');t.mock.timers.tick(500);assert.equal(hover.active,'');
  hover.show('/bank');hover.enter('/sales',{x:100,y:300});hover.close('/bank');t.mock.timers.tick(500);assert.equal(hover.active,'');
  hover.enter('/sales',{x:100,y:300});hover.cancel();t.mock.timers.tick(500);assert.equal(hover.active,'');
});
