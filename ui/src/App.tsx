import { Routes, Route, Navigate } from "react-router-dom";
import { Layout } from "./components/Layout";
import { Login } from "./pages/Login";
import { Dashboard } from "./pages/Dashboard";
import { Groups } from "./pages/Groups";
import { GroupDetail } from "./pages/GroupDetail";
import { Nodes } from "./pages/Nodes";
import { NodeDetail } from "./pages/NodeDetail";
import { Artifacts } from "./pages/Artifacts";
import { ArtifactBuilder } from "./pages/ArtifactBuilder";
import { ArtifactDetail } from "./pages/ArtifactDetail";
import { Deployments } from "./pages/Deployments";
import { Certificates } from "./pages/Certificates";
import { Import } from "./pages/Import";
import { Settings } from "./pages/Settings";
import { getToken } from "./api/client";

function RequireAuth({ children }: { children: React.ReactNode }) {
  if (!getToken()) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/"
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<Dashboard />} />
        <Route path="groups" element={<Groups />} />
        <Route path="groups/:id" element={<GroupDetail />} />
        <Route path="nodes" element={<Nodes />} />
        <Route path="nodes/:id" element={<NodeDetail />} />
        <Route path="artifacts" element={<Artifacts />} />
        <Route path="artifacts/new" element={<ArtifactBuilder />} />
        <Route path="artifacts/:id" element={<ArtifactDetail />} />
        <Route path="deployments" element={<Deployments />} />
        <Route path="certificates" element={<Certificates />} />
        <Route path="import" element={<Import />} />
        <Route path="settings" element={<Settings />} />
      </Route>
    </Routes>
  );
}
