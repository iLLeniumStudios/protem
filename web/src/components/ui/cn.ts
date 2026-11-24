import { clsx, type ClassValue } from 'clsx';
import { extendTailwindMerge } from 'tailwind-merge';

// tailwind-merge must know the custom font-size keys, or it treats `text-caption`
// as a colour and drops it next to `text-text-secondary`.
const merge = extendTailwindMerge({
  extend: {
    classGroups: {
      'font-size': [{ text: ['caption', 'label', 'body', 'title', 'heading', 'page', 'display', 'stat'] }],
    },
  },
});

export function cn(...inputs: ClassValue[]) {
  return merge(clsx(inputs));
}

export const focusRing =
  'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus';

export const colorTransition = 'transition-colors duration-150 motion-reduce:transition-none';
