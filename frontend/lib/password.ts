export const PASSWORD_MIN_LENGTH = 8;

export const PASSWORD_MAX_BYTES = 72;

function passwordByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function isPasswordTooLong(value: string): boolean {
  return passwordByteLength(value) > PASSWORD_MAX_BYTES;
}

export function isStrongPassword(value: string): boolean {
  return (
    !isPasswordTooLong(value) &&
    [...value].length >= PASSWORD_MIN_LENGTH &&
    /[a-zA-Z]/.test(value) &&
    /[0-9]/.test(value)
  );
}
