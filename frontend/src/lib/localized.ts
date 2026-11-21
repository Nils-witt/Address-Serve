import type { LocalizedText } from '../api/types';

/** The text in `language`, else in English, else in any language there is. */
export function localized(text: LocalizedText | undefined, language: string): string {
  if (text === undefined) return '';
  if (typeof text === 'string') return text;
  const base = language.split('-')[0];
  return text[base] ?? text.en ?? Object.values(text)[0] ?? '';
}
