import { BrowserRouter as Router, Routes, Route, Navigate } from "react-router-dom";
import Login from "./pages/Login";
import Subscriptions from "./pages/Subscriptions";
import Channels from "./pages/Channels";
import LLMs from "./pages/LLMs";
import Layout from "./components/Layout";
import ProtectedRoute from "./components/ProtectedRoute";

function App() {
  return (
    <Router>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route element={<ProtectedRoute />}>
          <Route element={<Layout />}>
            <Route path="/subscriptions" element={<Subscriptions />} />
            <Route path="/channels" element={<Channels />} />
            <Route path="/llms" element={<LLMs />} />
          </Route>
        </Route>
        <Route path="/dashboard" element={<Navigate to="/subscriptions" replace />} />
        <Route path="/config" element={<Navigate to="/channels" replace />} />
        <Route path="/" element={<Navigate to="/subscriptions" replace />} />
        <Route path="*" element={<Navigate to="/subscriptions" replace />} />
      </Routes>
    </Router>
  );
}

export default App;
