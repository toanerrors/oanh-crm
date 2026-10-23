interface EditorKey {
  key: string; ctrlKey: boolean; metaKey: boolean; altKey: boolean; shiftKey: boolean; isComposing: boolean; repeat: boolean;
}

export function editorShortcut(event: EditorKey): 'save' | 'preview' | undefined {
  if (!(event.ctrlKey || event.metaKey) || event.altKey || event.shiftKey || event.isComposing || event.repeat) return;
  if (event.key.toLowerCase() === 's') return 'save';
  if (event.key === 'Enter') return 'preview';
}
