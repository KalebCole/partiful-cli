import { EXIT } from './output.js';

/** Serialized error payload shape emitted by PartifulError.toJSON(). */
export interface PartifulErrorJSON {
  code: number;
  type: string;
  message: string;
  details?: unknown;
}

export class PartifulError extends Error {
  readonly exitCode: number;
  readonly type: string;
  readonly details: unknown;

  constructor(message: string, exitCode: number, type: string, details: unknown = null) {
    super(message);
    this.exitCode = exitCode;
    this.type = type;
    this.details = details;
  }

  toJSON(): PartifulErrorJSON {
    return {
      code: this.exitCode,
      type: this.type,
      message: this.message,
      ...(this.details ? { details: this.details } : {}),
    };
  }
}

export class ApiError extends PartifulError {
  constructor(message: string, details?: unknown) {
    super(message, EXIT.API_ERROR, 'api_error', details);
  }
}
export class AuthError extends PartifulError {
  constructor(message: string, details?: unknown) {
    super(message, EXIT.AUTH_ERROR, 'auth_error', details);
  }
}
export class ValidationError extends PartifulError {
  constructor(message: string, details?: unknown) {
    super(message, EXIT.VALIDATION_ERROR, 'validation_error', details);
  }
}
export class NotFoundError extends PartifulError {
  constructor(message: string, details?: unknown) {
    super(message, EXIT.NOT_FOUND, 'not_found', details);
  }
}
