import type { ReactNode } from "react";

export type HomePanelId = "sessions" | "tools" | "harnesses" | "skills" | "phone" | "control" | "secrets";
export type PanelFeedbackTone = "progress" | "success" | "warning" | "error";

export type PanelFeedbackState = {
  tone: PanelFeedbackTone;
  message: string;
  actionLabel?: string;
  actionToolId?: string;
};

export function HomePanel({ title, description, actions, className = "", feedback, onDismissFeedback, onFeedbackAction, children }: {
  title?: string;
  description?: ReactNode;
  actions?: ReactNode;
  className?: string;
  feedback?: PanelFeedbackState;
  onDismissFeedback?: () => void;
  onFeedbackAction?: () => void;
  children: ReactNode;
}) {
  return <section className={`tools-panel home-panel ${className}`}>
    {title && <div className="tools-header">
      <div><h2>{title}</h2>{description && <p className="muted">{description}</p>}</div>
      {actions}
    </div>}
    {children}
    <PanelFeedback feedback={feedback} onDismiss={onDismissFeedback} onAction={onFeedbackAction} />
  </section>;
}

export function PanelFeedback({ feedback, onDismiss, onAction }: {
  feedback?: PanelFeedbackState;
  onDismiss?: () => void;
  onAction?: () => void;
}) {
  if (!feedback) return null;
  const alert = feedback.tone === "warning" || feedback.tone === "error";
  return <div className={`panel-feedback ${feedback.tone}`} role={alert ? "alert" : "status"} aria-live={alert ? "assertive" : "polite"}>
    {feedback.tone === "progress" && <span className="spinner" aria-hidden="true" />}
    <span className="panel-feedback-message">{feedback.message}</span>
    {feedback.actionLabel && onAction && <button className="link-button" onClick={onAction}>{feedback.actionLabel}</button>}
    {onDismiss && <button className="icon-button" aria-label="Dismiss panel message" title="Dismiss" onClick={onDismiss}>x</button>}
  </div>;
}