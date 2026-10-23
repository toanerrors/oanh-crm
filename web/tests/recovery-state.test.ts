import test from 'node:test';
import assert from 'node:assert/strict';
import { restoreConfirmation,currentBackupState } from '../src/features/workspace/recovery-state.ts';
import type { RestorePreview,BackupOverview } from '../src/features/workspace/recovery-state.ts';
const preview:RestorePreview={id:'stage',expiresAt:'2026-10-24T12:30:00Z',current:{companyId:'company',companyName:'Công ty kiểm thử',epoch:'new',revision:9,documents:10,posted:8,attachments:2},manifest:{companyId:'company',companyName:'Công ty kiểm thử',epoch:'old',revision:4,createdAt:'2026-10-24T11:00:00Z',documents:4,posted:2,debit:'9007199254740993',credit:'9007199254740993'}};
test('khôi phục cần đúng tên, lý do, xác nhận và kết quả kiểm tra còn hạn',()=>{
 const now=Date.parse('2026-10-24T12:00:00Z');
 assert.equal(restoreConfirmation(preview,'Công ty kiểm thử','Đối chiếu sổ',true,now),true);
 assert.equal(restoreConfirmation(preview,'Công ty khác','Đối chiếu sổ',true,now),false);
 assert.equal(restoreConfirmation(preview,'Công ty kiểm thử','  ',true,now),false);
 assert.equal(restoreConfirmation(preview,'Công ty kiểm thử','Đối chiếu sổ',false,now),false);
 assert.equal(restoreConfirmation(preview,'Công ty kiểm thử','Đối chiếu sổ',true,Date.parse(preview.expiresAt)),false);
 assert.equal(restoreConfirmation(null,'Công ty kiểm thử','Đối chiếu sổ',true,now),false);
});
test('trạng thái bảo vệ không nhận bản sao của epoch trước làm bản hiện tại',()=>{
 const data={current:preview.current,latest:{verifiedAt:'2026-10-24T12:00:00Z',sha256:'verified',manifest:preview.manifest}} as Pick<BackupOverview,'current'|'latest'>;
 assert.equal(currentBackupState(data),'missing');
 assert.equal(currentBackupState({...data,latest:{...data.latest!,manifest:{...preview.manifest,epoch:'new'}}}),'changed');
 assert.equal(currentBackupState({...data,latest:{...data.latest!,manifest:{...preview.manifest,epoch:'new',revision:9}}}),'current');
 assert.equal(currentBackupState({...data,latest:null}),'missing');
});
