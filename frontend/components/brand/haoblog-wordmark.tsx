export function HaoBlogWordmark({
  className = "h-12 w-auto",
  label = "HaoBlog",
  gradientId = "haoblog-wordmark-gradient",
}: {
  className?: string;
  label?: string;
  gradientId?: string;
}) {
  return (
    <svg
      viewBox="-10 -10 355 103"
      role="img"
      aria-label={label}
      className={className}
    >
      <defs>
        <linearGradient
          id={gradientId}
          gradientUnits="userSpaceOnUse"
          x1="0"
          y1="42"
          x2="335"
          y2="42"
        >
          <stop offset="0" stopColor="#58ABF0" />
          <stop offset="1" stopColor="#1A63C6" />
        </linearGradient>
      </defs>
      <g
        fill="none"
        stroke={`url(#${gradientId})`}
        strokeWidth="10"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path d="M5 5V55M45 5V55M5 30H45" />
        <circle cx="80.5" cy="38.5" r="17.5" />
        <path d="M98 21V56" />
        <circle cx="133.5" cy="38.5" r="17.5" />
        <path d="M169 5V55M169 5H185A11 11 0 0 1 185 27H169M169 27H188A14.5 14.5 0 0 1 188 56H169" />
        <path d="M220 5V55" />
        <circle cx="254.5" cy="38.5" r="17.5" />
        <circle cx="307.5" cy="38.5" r="17.5" />
        <path d="M325 21V64C325 76 317 80 305 77" />
      </g>
    </svg>
  );
}
