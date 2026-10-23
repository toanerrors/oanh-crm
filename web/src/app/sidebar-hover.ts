export interface HoverPoint { x: number; y: number }
interface PanelBounds { left: number; top: number; bottom: number }

export function aimsAtSidebarPanel(origin: HoverPoint | undefined, point: HoverPoint, panel: PanelBounds | undefined): boolean {
  if (!origin || !panel || point.x <= origin.x || origin.x >= panel.left || point.x > panel.left) return false;
  const fraction = (point.x - origin.x) / (panel.left - origin.x);
  const top = origin.y + (panel.top - 12 - origin.y) * fraction;
  const bottom = origin.y + (panel.bottom + 12 - origin.y) * fraction;
  return point.y >= top && point.y <= bottom;
}

export class SidebarHover {
  active = '';
  private pending = '';
  private origin?: HoverPoint;
  private timer?: ReturnType<typeof setTimeout>;
  private changed: (path: string) => void;
  constructor(changed: (path: string) => void) { this.changed = changed; }
  cancel() { clearTimeout(this.timer); this.timer = undefined; this.pending = ''; }
  show(path: string) {
    this.cancel();
    this.origin = undefined;
    if (this.active !== path) { this.active = path; this.changed(path); }
  }
  enter(path: string, point: HoverPoint, panel?: PanelBounds) {
    this.cancel();
    if (path === this.active) return;
    const delay = this.active ? aimsAtSidebarPanel(this.origin, point, panel) ? 300 : 100 : 80;
    this.pending = path;
    this.timer = setTimeout(() => this.show(path), delay);
  }
  track(path: string, point: HoverPoint) { if (path === this.active) this.origin = point; }
  leave(path: string, point?: HoverPoint) {
    if (path !== this.active && path !== this.pending) return;
    this.cancel();
    if (path === this.active && !this.origin) this.origin = point;
    const active = this.active;
    this.timer = setTimeout(() => this.close(active), 240);
  }
  keep(path: string) { if (path === this.active) this.cancel(); }
  close(path: string) { if (path === this.active) this.show(''); else if (path === this.pending) this.cancel(); }
}
