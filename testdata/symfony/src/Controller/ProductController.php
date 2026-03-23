<?php

namespace App\Controller;

use Symfony\Bundle\FrameworkBundle\Controller\AbstractController;
use Symfony\Component\HttpFoundation\JsonResponse;
use Symfony\Component\HttpFoundation\Request;
use Symfony\Component\HttpFoundation\Response;
use App\Repository\ProductRepository;
use App\Service\NotificationService;
use App\Entity\Product;

class ProductController extends AbstractController
{
    public function __construct(
        private ProductRepository $repo,
        private NotificationService $notifier
    ) {}

    public function index(): JsonResponse
    {
        // Injected service method with return type
        $allProducts = $this->repo->findAll();

        // Typed return from repository
        $first = $this->repo->find(1);

        // Response creation
        $response = new JsonResponse(['products' => $allProducts]);

        return $response;
    }

    public function show(Request $request): Response
    {
        // Request parameter access
        $id = $request->get('id');

        // Method on injected service
        $this->notifier->notify('Product viewed');

        // Entity construction and method chain
        $product = new Product();
        $product->setName('Test');
        $productName = $product->getName();

        // Typed return from repository
        $found = $this->repo->find(1);

        // Array shape
        $data = ['id' => 1, 'name' => 'Test Product', 'price' => 9.99];

        return new Response('OK');
    }
}
