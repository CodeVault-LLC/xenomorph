import { cn } from "@/lib/utils"

type LogoProps = {
  className?: string
  withFrame?: boolean
}

const SILHOUETTE_PATH =
  "M128 64 C88 70 78 84 74 100 C56 108 48 134 60 152 C72 142 78 130 90 122 C96 148 102 166 115 175 C121 184 124 192 128 196 C132 192 135 184 141 175 C154 166 160 148 166 122 C178 130 184 142 196 152 C208 134 200 108 182 100 C178 84 168 70 128 64 Z M 99 124 C 109 121 121 130 121 135 C 113 138 95 131 99 124 Z M 157 124 C 147 121 135 130 135 135 C 143 138 161 131 157 124 Z"

const FRAME_PATH =
  "M128 18 L223.26 73 L223.26 183 L128 248 L32.74 183 L32.74 73 Z"

export function Logo({ className, withFrame = true }: LogoProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 256 256"
      role="img"
      aria-label="Xenomorph logo"
      className={cn("select-none", className)}
    >
      {withFrame ? (
        <path
          d={FRAME_PATH}
          fill="none"
          stroke="currentColor"
          strokeWidth={9}
          strokeLinejoin="round"
          strokeLinecap="round"
          opacity={0.32}
        />
      ) : null}
      <path fill="currentColor" fillRule="evenodd" d={SILHOUETTE_PATH} />
    </svg>
  )
}
