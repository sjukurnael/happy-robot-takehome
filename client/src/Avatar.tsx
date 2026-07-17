import { colorForClientId, initialsFor } from './identity'

export function Avatar({
  name,
  size = 22,
  ringColor,
  className,
}: {
  name: string
  size?: number
  ringColor?: string
  className?: string
}) {
  return (
    <span
      className={`avatar${className ? ` ${className}` : ''}`}
      title={name}
      style={{
        width: size,
        height: size,
        fontSize: Math.max(8, size * 0.42),
        background: colorForClientId(name),
        boxShadow: ringColor ? `0 0 0 2px ${ringColor}` : undefined,
      }}
    >
      {initialsFor(name)}
    </span>
  )
}
