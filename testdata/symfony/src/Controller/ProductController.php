<?php

namespace App\Controller;

use Symfony\Bundle\FrameworkBundle\Controller\AbstractController;
use Symfony\Component\HttpFoundation\JsonResponse;
use Symfony\Component\HttpFoundation\Request;
use Symfony\Component\HttpFoundation\Response;
use App\Service\NotificationService;

class ProductController extends AbstractController
{
    public function __construct(
        private NotificationService $notifier
    ) {}

    public function index(): JsonResponse
    {
        return $this->json(['products' => []]);
    }

    public function show(Request $request): Response
    {
        $id = $request->get('id');

        $this->notifier->notify('Product viewed');

        return $this->render('product/show.html.twig', ['id' => $id]);
    }
}
