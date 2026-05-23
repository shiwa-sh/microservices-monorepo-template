import { redirect } from "next/navigation";
// Default landing — route groups don't define a path segment, so the bare
// "/" is unrouted unless we redirect.
export default function Index() {
  redirect("/landing");
}
