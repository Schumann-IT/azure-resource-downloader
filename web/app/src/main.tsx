import React from 'react';
import ReactDOM from 'react-dom/client';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import App from './App';
import DiffPage from './pages/DiffPage';
import ResourcePage from './pages/ResourcePage';
import TenantPage from './pages/TenantPage';
import TenantsPage from './pages/TenantsPage';
import './index.css';

const router = createBrowserRouter([
  {
    path: '/',
    element: <App />,
    children: [
      { index: true, element: <TenantsPage /> },
      { path: 'tenant/:tenant', element: <TenantPage /> },
      { path: 'tenant/:tenant/:provider/:type/:slug', element: <ResourcePage /> },
      { path: 'diff', element: <DiffPage /> },
    ],
  },
]);

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <RouterProvider router={router} />
  </React.StrictMode>,
);
