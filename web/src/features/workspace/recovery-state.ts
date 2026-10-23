export interface BackupManifest {
 companyId: string; companyName: string; epoch: string; revision: number; createdAt: string; documents: number; posted: number; debit: string; credit: string;
 appVersion?: string; schemaVersion?: number; attachments?: number; firstDate?: string; lastDate?: string;
}
export interface BackupRecord {
 id: string; filename: string; size: number; sha256?: string; verifiedAt?: string; kind: string; actorName?: string; note?: string; manifest: BackupManifest;
}
export interface RecoveryCurrent { companyId: string; companyName: string; epoch: string; revision: number; documents: number; posted: number; attachments: number }
export interface RestorePreview { id: string; manifest: BackupManifest; current: RecoveryCurrent; expiresAt: string }
export interface BackupSchedule { enabled: boolean; version: number; time: string; timeZone: string; retention: string; updatedAt?: string; updatedBy?: string }
export interface BackupOverview {
 items: BackupRecord[]; total: number; count: number; page: number; pageSize: number; totalBytes: number; unreadable: number; latest: BackupRecord | null; current: RecoveryCurrent; schedule: BackupSchedule; nextScheduledAt: string;
 job: { status?: string; message?: string; scheduledFor?: string; finishedAt?: string; attempts?: number };
 recoveryEvents: {id: string; phase: string; actorName: string; startedAt: string; finishedAt?: string; snapshotCreatedAt: string; reason: string; preRestoreBackup: string}[];
}
export function restoreConfirmation(preview: RestorePreview | null, company: string, reason: string, acknowledged: boolean, now: number) {
 return Boolean(preview && company === preview.current.companyName && reason.trim().length >= 3 && reason.trim().length <= 500 && acknowledged && Date.parse(preview.expiresAt) > now);
}
export function currentBackupState(overview: Pick<BackupOverview,'latest'|'current'>): 'missing' | 'changed' | 'current' {
 if (!overview.latest || !overview.latest.verifiedAt || !overview.latest.sha256 || overview.latest.manifest.epoch !== overview.current.epoch) return 'missing';
 return overview.latest.manifest.revision === overview.current.revision ? 'current' : 'changed';
}
