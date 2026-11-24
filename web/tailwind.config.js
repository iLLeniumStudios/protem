const color = (name) => `var(--color-${name})`;
const tone = (name) => ({
  DEFAULT: color(name),
  ink: color(`${name}-ink`),
  soft: color(`${name}-soft`),
  text: color(`${name}-text`),
});
const text = (name) => [
  `var(--text-${name})`,
  {
    lineHeight: `var(--text-${name}--line-height)`,
    letterSpacing: `var(--text-${name}--letter-spacing)`,
    fontWeight: `var(--text-${name}--font-weight)`,
  },
];

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        bg: color('bg'),
        surface: color('surface'),
        subtle: color('subtle'),
        muted: color('muted'),
        border: color('border'),
        'border-strong': color('border-strong'),
        text: color('text'),
        'text-secondary': color('text-secondary'),
        'text-tertiary': color('text-tertiary'),
        accent: {
          DEFAULT: color('accent'),
          hover: color('accent-hover'),
          ink: color('accent-ink'),
          soft: color('accent-soft'),
          text: color('accent-text'),
        },
        focus: color('focus'),
        success: tone('success'),
        warning: tone('warning'),
        danger: { ...tone('danger'), hover: color('danger-hover') },
        info: tone('info'),
        scrim: color('scrim'),
      },
      fontSize: {
        caption: text('caption'),
        label: text('label'),
        body: text('body'),
        title: text('title'),
        heading: text('heading'),
        page: text('page'),
        display: text('display'),
        stat: text('stat'),
      },
      borderRadius: {
        sm: 'var(--radius-sm)',
        md: 'var(--radius-md)',
        lg: 'var(--radius-lg)',
        xl: 'var(--radius-xl)',
        full: 'var(--radius-full)',
      },
      fontFamily: {
        sans: 'var(--font-ui)',
        mono: 'var(--font-mono)',
      },
      maxHeight: {
        dialog: '85dvh',
        popover: 'var(--radix-select-content-available-height)',
      },
      minWidth: {
        trigger: 'var(--radix-select-trigger-width)',
      },
    },
  },
  plugins: [
    ({ addVariant }) => {
      addVariant('coarse', '@media (pointer: coarse)');
    },
  ],
};
