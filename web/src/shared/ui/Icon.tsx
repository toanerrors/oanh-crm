import type { CSSProperties } from 'react';

const paths = {
  grid: 'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z',
  cart: 'M3 3h2l2.4 12h11.2L21 7H6M9 20h.01M18 20h.01',
  bag: 'M5 7h14l1 14H4L5 7ZM9 8V6a3 3 0 0 1 6 0v2',
  wallet: 'M20 8V5H5a2 2 0 0 0 0 4h16v11H5a2 2 0 0 1-2-2V7M16 13h5v4h-5z',
  box: 'm12 3 9 5v9l-9 5-9-5V8l9-5ZM3 8l9 5 9-5M12 13v9M7.5 5.5l9 5',
  users: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M16 3a4 4 0 0 1 0 8M22 21v-2a4 4 0 0 0-3-3.9M13 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z',
  book: 'M4 3h15v18H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2ZM6 3v18M10 8h5M10 12h5',
  chart: 'M4 3v18h17M9 16v-5M14 16V7M19 16v-8',
  folder: 'M3 7V4h6l3 3h9v13H3V7Z',
  settings: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8ZM9 3h6l1 3 3 1 2 5-2 5-3 1-1 3H9l-1-3-3-1-2-5 2-5 3-1 1-3Z',
  help: 'M9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3M12 17h.01M22 12A10 10 0 1 1 2 12a10 10 0 0 1 20 0Z',
  search: 'm21 21-5-5M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0Z',
  bell: 'M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4',
  chevron: 'm9 5 7 7-7 7',
  down: 'm6 9 6 6 6-6',
  plus: 'M12 5v14M5 12h14',
  arrow: 'M5 12h14m-6-6 6 6-6 6',
  upRight: 'M7 17 17 7M7 7h10v10',
  download: 'M12 3v12m-5-5 5 5 5-5M4 16v5h16v-5',
  calendar: 'M8 2v5M16 2v5M3 10h18M3 5h18v17H3V5Z',
  file: 'M14 2H4v20h16V8l-6-6ZM14 2v6h6M8 13h8M8 17h5',
  check: 'm5 12 4 4L19 6',
  clock: 'M12 6v6l4 2M22 12A10 10 0 1 1 2 12a10 10 0 0 1 20 0Z',
  more: 'M5 12h.01M12 12h.01M19 12h.01',
  close: 'm6 6 12 12M6 18 18 6',
  menu: 'M3 6h18M3 12h18M3 18h18',
  arrowDown: 'M12 4v16m-6-6 6 6 6-6',
  arrowUp: 'M12 20V4m-6 6 6-6 6 6',
  refresh: 'M20 7v5h-5M4 17v-5h5M6 6a8 8 0 0 1 13 2M5 16a8 8 0 0 0 13 2',
  trash: 'M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7',
  print: 'M6 8V2h12v6M6 17H3V8h18v9h-3M6 14h12v8H6V14ZM17 11h.01',
  filter: 'M3 5h18M6 12h12M10 19h4',
  shield: 'm12 2 9 4v6c0 5-9 10-9 10S3 17 3 12V6l9-4Zm-5 10 3 3 7-7',
  spark: 'm12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5L12 3Z',
  building: 'M4 22V3h12v19M16 9h5v13M8 7h4M8 11h4M8 15h4M9 22v-3h3v3',
  mail: 'M 2 5h20v14H2V5Zm0 0 10 8L22 5',
  logout: 'M9 3H3v18h6M9 12h13m-5-5 5 5-5 5',
} as const;

export type IconName = keyof typeof paths;

export function Icon({ name, size = 20, className, style }: { name: IconName; size?: number; className?: string; style?: CSSProperties }) {
  return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.65" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" className={className} style={style}><path d={paths[name]} /></svg>;
}
