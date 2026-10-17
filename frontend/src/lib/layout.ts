// Panel sizes of the editor layout. The object list and the properties panel
// are resized with splitters; the preview keeps a minimum width and takes the
// rest.

/** Minimum width of the object list panel. */
export const LIST_MIN = 200;
/** Minimum width of the properties panel. */
export const PROPS_MIN = 220;
/** Width the preview keeps, whatever the panels ask for. */
export const CENTER_MIN = 460;
/** Two 6px splitter handles plus the 12px padding of the layout. */
export const LAYOUT_EXTRA = 2 * 6 + 12;

/** Widest the object list may be in a window `room` CSS pixels wide. */
export function maxListWidth(room: number, propsWidth: number): number {
  return Math.max(LIST_MIN, room - propsWidth - CENTER_MIN - LAYOUT_EXTRA);
}

/** Widest the properties panel may be in a window `room` CSS pixels wide. */
export function maxPropsWidth(room: number, listWidth: number): number {
  return Math.max(PROPS_MIN, room - listWidth - CENTER_MIN - LAYOUT_EXTRA);
}

/** Width of the object list after dragging its splitter `d` pixels right. */
export function dragList(room: number, listWidth: number, propsWidth: number, d: number): number {
  return Math.round(Math.min(maxListWidth(room, propsWidth), Math.max(LIST_MIN, listWidth + d)));
}

/** Width of the properties panel after dragging its splitter `d` pixels right
 * (dragging left makes it wider). */
export function dragProps(room: number, listWidth: number, propsWidth: number, d: number): number {
  return Math.round(Math.min(maxPropsWidth(room, listWidth), Math.max(PROPS_MIN, propsWidth - d)));
}

/** Minimum window width that fits both panels, the preview and the handles. */
export function minLayoutWidth(listWidth: number, propsWidth: number): number {
  return listWidth + propsWidth + CENTER_MIN + LAYOUT_EXTRA;
}