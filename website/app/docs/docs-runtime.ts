declare global {
  // Used by JSX endpoint examples such as /admin/keys/{id}/enable.
  // The value intentionally includes braces so the rendered documentation stays literal.
  var id: string;
}

globalThis.id = "{id}";

export {};
