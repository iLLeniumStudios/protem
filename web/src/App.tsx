import { Navigate, Route, Routes } from 'react-router-dom';
import { DataProvider } from './app/data';
import { NotFound } from './app/NotFound';
import { REDIRECTS } from './app/roles';
import { Shell } from './app/Shell';
import AccessTimeline from './pages/admin/AccessTimeline';
import Overview from './pages/admin/Overview';
import Approvals from './pages/Approvals';
import Audit from './pages/Audit';
import Fleet from './pages/Fleet';
import GrantDetail from './pages/GrantDetail';
import Home from './pages/Home';
import MyAccess from './pages/MyAccess';
import ProfileDetail from './pages/ProfileDetail';
import ProfileEditor from './pages/ProfileEditor';
import Profiles from './pages/Profiles';
import RequestAccess from './pages/RequestAccess';
import RequestDetail from './pages/RequestDetail';
import RequestForm from './pages/RequestForm';

export function App() {
  return (
    <DataProvider>
      <Routes>
        <Route element={<Shell />}>
          <Route index element={<Home />} />
          <Route path="request" element={<RequestAccess />} />
          <Route path="request/:ns/:profile" element={<RequestForm />} />
          <Route path="requests" element={<MyAccess />} />
          <Route path="requests/:ns/:name" element={<RequestDetail />} />
          <Route path="grants/:ns/:name" element={<GrantDetail />} />
          <Route path="approvals" element={<Approvals />} />
          <Route path="admin" element={<Overview />} />
          <Route path="timeline" element={<AccessTimeline />} />
          <Route path="profiles" element={<Profiles />} />
          <Route path="profiles/new" element={<ProfileEditor />} />
          <Route path="profiles/:ns/:name" element={<ProfileDetail />} />
          <Route path="profiles/:ns/:name/edit" element={<ProfileEditor />} />
          <Route path="clusters" element={<Fleet />} />
          <Route path="audit" element={<Audit />} />
          {Object.entries(REDIRECTS).map(([from, to]) => (
            <Route key={from} path={from.slice(1)} element={<Navigate replace to={to} />} />
          ))}
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
    </DataProvider>
  );
}
