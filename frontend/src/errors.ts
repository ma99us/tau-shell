export function errorDetails(error: unknown): { message: string; stack: string } {
  if (error instanceof Error) return { message: error.message, stack: error.stack ?? "" };
  return { message: String(error), stack: "" };
}

export function reportFrontendFailure(source: string, error: unknown, stack = ""): string {
  const details = errorDetails(error);
  const fullStack = stack || details.stack;
  const logger = window.go?.main?.App?.LogFrontendFailure;
  if (logger) {
    void logger(source, details.message, fullStack).catch((logError) => {
      console.error("TauShell could not write a frontend failure to its diagnostic log.", logError);
    });
  } else {
    console.error(`TauShell frontend failure (${source}):`, error);
  }
  return details.message;
}

// A resize can race with session stop; that is not a user-facing failure.
export function isStoppedSessionError(error: unknown): boolean {
  return errorDetails(error).message.includes("is not running");
}
