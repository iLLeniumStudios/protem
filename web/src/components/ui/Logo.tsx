import type { SVGProps } from 'react';

/**
 * The Protem symbol in its small-size cut (docs/brand/logo/protem-symbol-small.svg), meant for 16 to 24 px.
 * It takes its colour from the text colour.
 */
export function ProtemMark(props: SVGProps<SVGSVGElement>) {
  return (
    <svg viewBox="0 0 256 256" fill="currentColor" {...props}>
      <path
        fillRule="evenodd"
        d="M28 28 L124 28 L124 74 L74 74 L74 228 L28 228 Z M228 28 L228 228 L104 228 L104 182 L182 182 L182 28 Z"
      />
    </svg>
  );
}
