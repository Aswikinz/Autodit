import { useEffect, useRef, useState, type ReactNode } from "react";
import { X, ArrowUpRight, LoaderCircle } from "lucide-react";
import { label } from "../lib/client";

export function Badge({ value }: { value: string }) {
  return <span className={`badge ${value}`}>{label(value)}</span>;
}
export function Spinner() {
  return (
    <div className="loading">
      <LoaderCircle size={20} className="spin" /> Loading workspace…
    </div>
  );
}
export function ErrorBox({ children }: { children: ReactNode }) {
  return (
    <div className="error" role="alert">
      {children}
    </div>
  );
}
export function Empty({
  title,
  children,
  action,
}: {
  title: string;
  children: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-mark">
        <ArrowUpRight size={28} />
      </div>
      <h3>{title}</h3>
      <p>{children}</p>
      {action}
    </div>
  );
}
export function Drawer({
  title,
  kicker,
  onClose,
  children,
  fullscreen = false,
}: {
  title: string;
  kicker: string;
  onClose: () => void;
  children: ReactNode;
  fullscreen?: boolean;
}) {
  const dialog = useRef<HTMLElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    dialog.current?.focus();
    return () => {
      document.body.style.overflow = overflow;
      previous?.focus();
    };
  }, []);
  return (
    <div className="overlay" role="presentation" onClick={onClose}>
      <section
        ref={dialog}
        className={fullscreen ? "drawer workbench" : "drawer"}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (e.key === "Escape") onClose();
          if (e.key === "Tab") {
            const items = Array.from(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], [tabindex="0"]') ?? []).filter(el => el.getClientRects().length);
            const first = items[0];
            const last = items.at(-1);
            if (e.shiftKey && (document.activeElement === first || document.activeElement === dialog.current)) { e.preventDefault(); last?.focus(); }
            else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first?.focus(); }
          }
        }}
        tabIndex={-1}
      >
        <header>
          <div>
            <span className="eyebrow">{kicker}</span>
            <h2>{title}</h2>
          </div>
          <button
            className="icon-button"
            aria-label="Close detail"
            onClick={onClose}
          >
            <X />
          </button>
        </header>
        <div className="drawer-body">{children}</div>
      </section>
    </div>
  );
}

export function Help({ title, children }: { title: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return <div className="help-guide">
    <button type="button" className="button" aria-expanded={open} onClick={() => setOpen(!open)}>Help: {title}</button>
    {open && <section className="notice" aria-label={`${title} guide`}>{children}</section>}
  </div>;
}
export function Metric({
  label: caption,
  value,
  note,
}: {
  label: string;
  value: string | number;
  note: string;
}) {
  return (
    <div className="metric">
      <span>{caption}</span>
      <strong>{value}</strong>
      <small>{note}</small>
    </div>
  );
}
export function SectionTitle({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="section-heading">
      <div>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {action}
    </div>
  );
}
