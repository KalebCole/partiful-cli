import { EXIT } from './output.js';

export class PartifulError extends Error {
  constructor(message, exitCode, type, details = null) {
    super(message);
    this.exitCode = exitCode;
    this.type = type;
    this.details = details;
  }
  toJSON() {
    return {
      code: this.exitCode, type: this.type, message: this.message,
      ...(this.details ? { details: this.details } : {}),
    };
  }
}

export class ApiError extends PartifulError {
  constructor(message, details) { super(message, EXIT.API_ERROR, 'api_error', details); }
}
export class AuthError extends PartifulError {
  constructor(message, details) { super(message, EXIT.AUTH_ERROR, 'auth_error', details); }
}
export class ValidationError extends PartifulError {
  constructor(message, details) { super(message, EXIT.VALIDATION_ERROR, 'validation_error', details); }
}
export class NotFoundError extends PartifulError {
  constructor(message, details) { super(message, EXIT.NOT_FOUND, 'not_found', details); }
}
