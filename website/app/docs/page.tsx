import type { Metadata } from "next";
import "./docs-runtime";
import { docPages } from "./docs-content";
import { DocsShell } from "./docs-shell";

export const metadata: Metadata = {
  title: "Documentation",
  description: "Complete ModelMux documentation for installation, configuration, routing, operations, security, and API reference.",
};

export default function DocsPage() {
  return <DocsShell page={docPages.overview} />;
}
