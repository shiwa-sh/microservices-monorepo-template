import type { ButtonHTMLAttributes } from "react";
import { cn } from "../cn";

type Variant = "primary" | "secondary" | "ghost";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
};

const variants: Record<Variant, string> = {
  primary: "bg-brand-600 text-white hover:bg-brand-700",
  secondary: "bg-slate-100 text-slate-900 hover:bg-slate-200",
  ghost: "bg-transparent text-slate-900 hover:bg-slate-100",
};

export function Button({ className, variant = "primary", ...rest }: Props) {
  return (
    <button
      type="button"
      className={cn(
        "inline-flex items-center justify-center rounded-md px-4 py-2 text-sm font-medium",
        "transition-colors disabled:opacity-50 disabled:pointer-events-none",
        variants[variant],
        className,
      )}
      {...rest}
    />
  );
}
